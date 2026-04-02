package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"clawdesk/src/config"

	"go.yaml.in/yaml/v3"
)

// PropDef 参数属性定义
type PropDef struct {
	Type        string         `yaml:"type" json:"type"`
	Description string         `yaml:"description" json:"description"`
	Items       map[string]any `yaml:"items,omitempty" json:"items,omitempty"` // array 类型的元素定义
	Enum        []string       `yaml:"enum,omitempty" json:"enum,omitempty"`   // 枚举值
}

// ToolParam 工具参数 schema（OpenAI function calling 格式）
type ToolParam struct {
	Type       string             `yaml:"type" json:"type"`
	Properties map[string]PropDef `yaml:"properties" json:"properties"`
	Required   []string           `yaml:"required" json:"required"`
}

// ToolExecute 工具执行方式
type ToolExecute struct {
	Type    string `yaml:"type" json:"type"`       // "command" | "builtin" | "mcp"
	Command string `yaml:"command" json:"command"` // type=command 时的 shell 命令模板
}

// Tool 工具定义
type Tool struct {
	Name        string      `yaml:"name" json:"name"`
	Description string      `yaml:"description" json:"description"`
	Parameters  ToolParam   `yaml:"parameters" json:"parameters"`
	Execute     ToolExecute `yaml:"execute" json:"execute"`
}

// Skill 技能定义
type Skill struct {
	Name          string     `yaml:"name" json:"name"`
	DisplayName   string     `yaml:"displayName" json:"displayName"`
	Description   string     `yaml:"description" json:"description"`
	Version       string     `yaml:"version" json:"version"`
	Enabled       bool       `yaml:"enabled" json:"enabled"`
	Builtin       bool       `yaml:"-" json:"builtin"`                     // 内置技能不可删除
	Deferred      bool       `yaml:"-" json:"deferred,omitempty"`          // 延迟加载，默认只发送激活工具
	Type          string     `yaml:"type,omitempty" json:"type,omitempty"` // "" = agent skill, "mcp" = MCP server
	Format        string     `yaml:"-" json:"format,omitempty"`            // "yaml" | "skillmd"，加载来源格式
	MCP           *MCPConfig `yaml:"mcp,omitempty" json:"mcp,omitempty"`   // MCP 配置
	Content       string     `yaml:"-" json:"content,omitempty"`           // SKILL.md 正文（注入系统提示词）
	Tools         []Tool     `yaml:"tools" json:"tools"`
	Order         int        `yaml:"-" json:"-"`                       // 排序序号（内置在前，自定义按安装顺序）
	SecurityLevel string     `yaml:"-" json:"securityLevel,omitempty"` // safe | caution | ""
	SecurityNote  string     `yaml:"-" json:"securityNote,omitempty"`  // 安全审查备注
}

// LLMCallFunc LLM 调用函数类型（用于压缩器等内部功能）
type LLMCallFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// VetToolCall 安全审查中 LLM 发起的工具调用
type VetToolCall struct {
	ID       string
	Name     string
	Arguments string
}

// VetLLMResponse 安全审查 LLM 响应
type VetLLMResponse struct {
	Content   string
	ToolCalls []VetToolCall
}

// VetLLMCallFunc 带工具调用的 LLM 请求函数（用于 Skill Vetter 安全审查）
type VetLLMCallFunc func(ctx context.Context, messages []VetMessage, tools []map[string]any) (*VetLLMResponse, error)

// VetMessage 安全审查消息
type VetMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCalls  []VetToolCall `json:"tool_calls,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Name       string `json:"name,omitempty"`
}

// Manager 技能管理器
type Manager struct {
	mu                sync.RWMutex
	skills            map[string]*Skill
	builtinExecutors  map[string]func(args map[string]any) ToolResult
	mcpClients        map[string]*MCPClient // skillName -> MCP client
	nextOrder         int                   // 自增序号，用于安装排序
	llmCall           LLMCallFunc           // LLM 调用（用于压缩器等）
	vetLLMCall        VetLLMCallFunc        // 带工具调用的 LLM 请求（用于 Skill Vetter 安全审查）
	lastSkillDirMod   time.Time             // skills 目录最后修改时间（跳过无变化的扫描）
	workspaceResolver func() string         // 返回当前会话的 workspace 目录
}

// SetWorkspaceResolver 设置工作区路径解析器
func (m *Manager) SetWorkspaceResolver(fn func() string) {
	m.workspaceResolver = fn
}

// resolveWorkspacePath 将相对路径解析到 workspace 目录
func (m *Manager) resolveWorkspacePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	if m.workspaceResolver != nil {
		if ws := m.workspaceResolver(); ws != "" {
			return filepath.Join(ws, path)
		}
	}
	return path
}

// getWorkspace 获取当前 workspace 目录
func (m *Manager) getWorkspace() string {
	if m.workspaceResolver != nil {
		return m.workspaceResolver()
	}
	return ""
}

// GetSkillByTool 根据工具名查找所属技能名
func (m *Manager) GetSkillByTool(toolName string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.skills {
		for _, t := range s.Tools {
			if t.Name == toolName {
				if s.DisplayName != "" {
					return s.DisplayName
				}
				return s.Name
			}
		}
	}
	return ""
}

// sanitizeOutput 将输出中的 workspace 绝对路径替换为相对路径
func (m *Manager) sanitizeOutput(text string) string {
	if ws := m.getWorkspace(); ws != "" {
		text = strings.ReplaceAll(text, ws+"/", "")
		text = strings.ReplaceAll(text, ws, ".")
	}
	return text
}

// NewManager 创建技能管理器
func NewManager(ctx context.Context, llmCall LLMCallFunc, vetLLMCall VetLLMCallFunc) *Manager {
	m := &Manager{
		skills:           make(map[string]*Skill),
		builtinExecutors: make(map[string]func(args map[string]any) ToolResult),
		mcpClients:       make(map[string]*MCPClient),
		nextOrder:        100, // 内置技能用 0~99，自定义从 100 开始
		llmCall:          llmCall,
		vetLLMCall:       vetLLMCall,
	}
	m.registerBuiltins()
	m.loadCustomSkills(ctx)
	return m
}

// VetResult 安全检查结果
type VetResult struct {
	Level string // "safe" | "caution" | ""
	Note  string // 审查备注
}

// VetSkill 使用 Skill Vetter 检查待安装技能的安全性
// skillDir 是技能目录的绝对路径，LLM 会通过工具自主扫描目录下所有文件
// 返回 VetResult + error（error 非 nil 表示被拒绝）
func (m *Manager) VetSkill(skillName, skillDir string) (VetResult, error) {
	// Skill Vetter 自身不检查
	if skillName == "skill-vetter" {
		return VetResult{Level: "safe", Note: "Skill Vetter 自身"}, nil
	}

	// 查找已安装的 Skill Vetter
	m.mu.RLock()
	vetter, ok := m.skills["skill-vetter"]
	m.mu.RUnlock()

	// 未安装 Skill Vetter → 自动从 ClawHub 下载安装
	if !ok || vetter.Content == "" {
		vetterName, err := InstallClawHubSkill("skill-vetter")
		if err != nil {
			fmt.Printf("自动安装 Skill Vetter 失败: %v，跳过安全检查\n", err)
			return VetResult{}, nil
		}
		dir := filepath.Join(skillsDir(), vetterName)
		s := m.loadSkillFromDir(dir)
		if s == nil || s.Content == "" {
			fmt.Println("Skill Vetter 加载失败，跳过安全检查")
			return VetResult{}, nil
		}
		m.mu.Lock()
		s.Builtin = false
		s.Order = m.nextOrder
		m.nextOrder++
		m.skills[s.Name] = s
		m.mu.Unlock()
		vetter = s
		fmt.Println("已自动安装 Skill Vetter")
	}

	if m.vetLLMCall == nil {
		return VetResult{}, nil
	}

	// 构建审查工具定义（只提供 list_directory 和 read_file）
	vetTools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "list_directory",
				"description": "列出指定目录下的文件和文件夹",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "目录路径"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "read_file",
				"description": "读取指定文件的文本内容",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "文件路径"},
					},
					"required": []string{"path"},
				},
			},
		},
	}

	// LLM 审查提示词
	systemPrompt := vetter.Content + fmt.Sprintf(`

你正在审查一个待安装的技能，技能目录: %s
请使用 list_directory 和 read_file 工具扫描该目录下的所有文件（包括子目录中的脚本、配置文件等），全面审查安全性。

审查要点：
1. SKILL.md / skill.yaml 中的命令和描述
2. 目录下所有脚本文件（.sh, .py, .js, .bat 等）的实际内容
3. 是否存在命令注入、数据外泄、恶意下载执行等风险
4. 是否有可疑的网络请求、文件系统越权访问

审查完成后，按以下格式回复最终结论（只回复一行）：
- 安全无风险: PASS:safe:简要说明
- 通过但有轻微风险: PASS:caution:风险说明
- 有明确安全风险: REJECT:拒绝原因`, skillDir)

	userPrompt := fmt.Sprintf("请审查技能: %s\n技能目录: %s\n\n请先用 list_directory 查看目录结构，再逐个 read_file 检查所有文件内容。", skillName, skillDir)

	// 构建消息列表
	messages := []VetMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	// Function Calling 循环（最多 10 轮）
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		resp, err := m.vetLLMCall(ctx, messages, vetTools)
		if err != nil {
			fmt.Printf("Skill Vetter LLM 调用失败: %v，跳过安全检查\n", err)
			return VetResult{}, nil
		}

		// 无工具调用，解析最终结果
		if len(resp.ToolCalls) == 0 {
			return parseVetResult(resp.Content)
		}

		// 将 assistant 消息（含 tool_calls）加入消息列表
		messages = append(messages, VetMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// 执行工具调用
		for _, tc := range resp.ToolCalls {
			output := m.executeVetTool(tc.Name, tc.Arguments, skillDir)
			messages = append(messages, VetMessage{
				Role:       "tool",
				Content:    output,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			})
		}
	}

	return VetResult{}, fmt.Errorf("安全审查工具调用次数超过限制")
}

// executeVetTool 执行安全审查工具（限制在技能目录内）
func (m *Manager) executeVetTool(name, argsJSON, skillDir string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}

	path, _ := args["path"].(string)
	if path == "" {
		return "错误: path 参数为空"
	}

	// 安全限制：路径必须在技能目录内
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Sprintf("路径错误: %v", err)
	}
	if !strings.HasPrefix(absPath, skillDir) {
		return fmt.Sprintf("安全限制: 只能访问技能目录 %s 内的文件", skillDir)
	}

	switch name {
	case "list_directory":
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return fmt.Sprintf("读取目录失败: %v", err)
		}
		var sb strings.Builder
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if entry.IsDir() {
				fmt.Fprintf(&sb, "[目录] %s\n", entry.Name())
			} else {
				fmt.Fprintf(&sb, "[文件] %s (%d bytes)\n", entry.Name(), info.Size())
			}
		}
		if sb.Len() == 0 {
			return "目录为空"
		}
		return sb.String()

	case "read_file":
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Sprintf("文件不存在: %v", err)
		}
		if info.Size() > 1*1024*1024 {
			return "文件过大（>1MB），跳过"
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Sprintf("读取失败: %v", err)
		}
		content := string(data)
		if len(content) > 8000 {
			content = content[:8000] + "\n... (内容被截断)"
		}
		return content

	default:
		return fmt.Sprintf("未知工具: %s", name)
	}
}

// parseVetResult 解析 LLM 返回的审查结论
func parseVetResult(text string) (VetResult, error) {
	result := strings.TrimSpace(text)
	upper := strings.ToUpper(result)

	if strings.Contains(upper, "REJECT") {
		reason := result
		if idx := strings.Index(upper, "REJECT:"); idx >= 0 {
			reason = strings.TrimSpace(result[idx+7:])
		}
		return VetResult{Level: "danger", Note: reason}, fmt.Errorf("技能安全检查未通过: %s", reason)
	}

	if strings.Contains(upper, "PASS") {
		level := "safe"
		note := ""
		if idx := strings.Index(upper, "PASS:"); idx >= 0 {
			parts := strings.SplitN(result[idx+5:], ":", 2)
			if len(parts) >= 1 {
				l := strings.TrimSpace(strings.ToLower(parts[0]))
				if l == "safe" || l == "caution" {
					level = l
				}
			}
			if len(parts) >= 2 {
				note = strings.TrimSpace(parts[1])
			}
		}
		return VetResult{Level: level, Note: note}, nil
	}

	return VetResult{}, fmt.Errorf("技能安全检查结果不明确: %s", result)
}

// skillsDir 技能存储目录
func skillsDir() string {
	return filepath.Join(config.GetConfigDir(), "skills")
}

// saveSecurityResult 保存安全检查结果到磁盘
func saveSecurityResult(skillName string, level, note string) {
	data, _ := json.Marshal(map[string]string{"level": level, "note": note})
	path := filepath.Join(skillsDir(), skillName, "_security.json")
	os.WriteFile(path, data, 0644)
}

// loadSecurityResult 从磁盘加载安全检查结果
func loadSecurityResult(skillName string) (string, string) {
	path := filepath.Join(skillsDir(), skillName, "_security.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	var result struct {
		Level string `json:"level"`
		Note  string `json:"note"`
	}
	if json.Unmarshal(data, &result) != nil {
		return "", ""
	}
	return result.Level, result.Note
}

// RegisterBuiltinExecutor 注册内置工具执行器
func (m *Manager) RegisterBuiltinExecutor(toolName string, fn func(args map[string]any) ToolResult) {
	m.builtinExecutors[toolName] = fn
}

// loadCustomSkills 从磁盘加载自定义技能（支持 skill.yaml 和 SKILL.md 两种格式）
func (m *Manager) loadCustomSkills(ctx context.Context) {
	dir := skillsDir()
	os.MkdirAll(dir, 0755)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, entry.Name())
		s := m.loadSkillFromDir(skillDir)
		if s == nil {
			continue
		}
		s.Builtin = false
		s.Order = m.nextOrder
		m.nextOrder++
		s.SecurityLevel, s.SecurityNote = loadSecurityResult(s.Name)
		m.skills[s.Name] = s

		// 自动连接已启用的 MCP 技能
		if s.Type == "mcp" && s.Enabled && s.MCP != nil {
			if err := m.connectMCP(ctx, s); err != nil {
				println("MCP 连接失败:", s.Name, err.Error())
			}
		}
	}
}

// loadSkillFromDir 从目录加载技能，优先 skill.yaml，其次 SKILL.md
func (m *Manager) loadSkillFromDir(dir string) *Skill {
	// 优先 skill.yaml
	yamlPath := filepath.Join(dir, "skill.yaml")
	if data, err := os.ReadFile(yamlPath); err == nil {
		var s Skill
		if err := yaml.Unmarshal(data, &s); err == nil {
			s.Format = "yaml"
			return &s
		}
	}

	// 其次 SKILL.md
	mdPath := filepath.Join(dir, "SKILL.md")
	if data, err := os.ReadFile(mdPath); err == nil {
		s := parseSkillMD(string(data), filepath.Base(dir))
		if s != nil {
			// 从 _meta.json 补充版本号
			metaPath := filepath.Join(dir, "_meta.json")
			if metaData, err := os.ReadFile(metaPath); err == nil {
				var meta struct {
					Version string `json:"version"`
				}
				if json.Unmarshal(metaData, &meta) == nil && meta.Version != "" {
					s.Version = meta.Version
				}
			}
			// 注册 executor：调用技能工具 = 返回 SKILL.md 内容
			m.registerSkillMDLoader(s)
			return s
		}
	}

	return nil
}

// registerSkillMDLoader 为 SKILL.md 技能注册 executor，调用时实时读取文件内容
func (m *Manager) registerSkillMDLoader(s *Skill) {
	if _, ok := m.builtinExecutors[s.Name]; ok {
		return
	}
	skillDir := filepath.Join(skillsDir(), s.Name)
	mdPath := filepath.Join(skillDir, "SKILL.md")
	m.builtinExecutors[s.Name] = func(args map[string]any) ToolResult {
		data, err := os.ReadFile(mdPath)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("读取 SKILL.md 失败: %v", err), Success: false}
		}
		_, body := splitFrontmatter(string(data))

		var sb strings.Builder
		// 将 SKILL.md 中的相对路径替换为绝对路径
		// 扫描技能目录下的子目录，将出现的 子目录名/ 替换为绝对路径
		absBody := body
		if subEntries, err := os.ReadDir(skillDir); err == nil {
			for _, entry := range subEntries {
				if entry.IsDir() {
					relPrefix := entry.Name() + "/"
					absPrefix := skillDir + "/" + relPrefix
					absBody = strings.ReplaceAll(absBody, relPrefix, absPrefix)
				}
			}
		}

		sb.WriteString(fmt.Sprintf(`=== 技能: %s ===

执行规则:
1. 严格按照技能说明中的流程和条件判断执行，不要跳过任何步骤
2. 需要查阅附属文档时，使用 read_file 工具读取对应路径
3. 需要执行命令或脚本时，使用 execute_command 工具

`, s.Name))
		sb.WriteString(absBody)

		// 列出技能目录下所有子目录中的文件路径（不加载内容，按需读取）
		var docs []string
		if subEntries, err := os.ReadDir(skillDir); err == nil {
			for _, sub := range subEntries {
				if !sub.IsDir() {
					continue
				}
				subDir := filepath.Join(skillDir, sub.Name())
				if files, err := os.ReadDir(subDir); err == nil {
					for _, f := range files {
						if !f.IsDir() {
							docs = append(docs, filepath.Join(subDir, f.Name()))
						}
					}
				}
			}
		}
		if len(docs) > 0 {
			sb.WriteString("\n\n附属文档（按需使用 read_file 读取）:\n")
			for _, d := range docs {
				sb.WriteString("- " + d + "\n")
			}
		}

		return ToolResult{Output: sb.String(), Success: true}
	}
}

// ReloadCustomSkills 重新加载磁盘上的自定义技能（新安装的技能会被发现）
func (m *Manager) ReloadCustomSkills(ctx context.Context) {
	dir := skillsDir()

	// modtime 守卫：目录未变化时跳过扫描
	info, err := os.Stat(dir)
	if err != nil {
		return
	}
	if !info.ModTime().After(m.lastSkillDirMod) {
		return
	}
	m.lastSkillDirMod = info.ModTime()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// 快照已有技能名（短暂持锁）
	m.mu.RLock()
	existing := make(map[string]bool, len(m.skills))
	for name := range m.skills {
		existing[name] = true
	}
	m.mu.RUnlock()

	// 收集新技能（不持锁）
	var newSkills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if existing[entry.Name()] {
			continue
		}
		skillDir := filepath.Join(dir, entry.Name())
		s := m.loadSkillFromDir(skillDir)
		if s == nil {
			continue
		}
		if existing[s.Name] {
			continue
		}
		newSkills = append(newSkills, s)
	}

	// 对新技能进行安全检查（不持锁，VetSkill 内部会获取读锁）
	var vetted []*Skill
	for _, s := range newSkills {
		skillDir := filepath.Join(dir, s.Name)
		vetResult, err := m.VetSkill(s.Name, skillDir)
		if err != nil {
			println("技能安全检查未通过:", s.Name, err.Error())
			os.RemoveAll(skillDir)
			continue
		}
		s.SecurityLevel = vetResult.Level
		s.SecurityNote = vetResult.Note
		saveSecurityResult(s.Name, vetResult.Level, vetResult.Note)
		vetted = append(vetted, s)
	}

	// 写入通过检查的技能
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range vetted {
		if _, ok := m.skills[s.Name]; ok {
			continue
		}
		s.Builtin = false
		s.Order = m.nextOrder
		m.nextOrder++
		m.skills[s.Name] = s

		if s.Type == "mcp" && s.Enabled && s.MCP != nil {
			if err := m.connectMCP(ctx, s); err != nil {
				println("MCP 连接失败:", s.Name, err.Error())
			}
		}
	}
}

// saveSkill 保存技能到磁盘
func (m *Manager) saveSkill(s *Skill) error {
	if s.Builtin {
		return nil // 内置技能不保存到磁盘
	}
	dir := filepath.Join(skillsDir(), s.Name)
	os.MkdirAll(dir, 0755)

	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "skill.yaml"), data, 0644)
}

// deleteSkillDir 删除技能目录
func (m *Manager) deleteSkillDir(name string) error {
	dir := filepath.Join(skillsDir(), name)
	return os.RemoveAll(dir)
}

// ===== 对外接口 =====

// List 获取所有技能（内置在前，自定义按安装顺序）
func (m *Manager) List() []Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Skill, 0, len(m.skills))
	for _, s := range m.skills {
		result = append(result, *s)
	}
	sort.Slice(result, func(i, j int) bool {
		// 内置技能优先
		if result[i].Builtin != result[j].Builtin {
			return result[i].Builtin
		}
		// 同类按安装顺序
		return result[i].Order < result[j].Order
	})
	return result
}

// Get 获取单个技能（SKILL.md 格式的技能实时读取文件内容）
func (m *Manager) Get(name string) *Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.skills[name]
	if !ok {
		return nil
	}
	// SKILL.md 格式实时从文件读取内容
	if s.Format == "skillmd" {
		mdPath := filepath.Join(skillsDir(), s.Name, "SKILL.md")
		if data, err := os.ReadFile(mdPath); err == nil {
			_, body := splitFrontmatter(string(data))
			s.Content = body
		}
	}
	return s
}

// InstallFromYAML 从 YAML 内容安装技能
func (m *Manager) InstallFromYAML(yamlContent string) error {
	var s Skill
	if err := yaml.Unmarshal([]byte(yamlContent), &s); err != nil {
		return fmt.Errorf("YAML 解析失败: %w", err)
	}
	if s.Name == "" {
		return fmt.Errorf("技能名称为空")
	}
	s.Enabled = true
	return m.Install(s)
}

// Install 安装新技能（Agent Skill）
func (m *Manager) Install(s Skill) error {
	// 安全检查（在获取锁之前，避免 VetSkill 内部读锁死锁）
	skillDir := filepath.Join(skillsDir(), s.Name)
	vetResult, err := m.VetSkill(s.Name, skillDir)
	if err != nil {
		return err
	}
	s.SecurityLevel = vetResult.Level
	s.SecurityNote = vetResult.Note
	saveSecurityResult(s.Name, vetResult.Level, vetResult.Note)

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.skills[s.Name]; ok && existing.Builtin {
		return fmt.Errorf("不能覆盖内置技能: %s", s.Name)
	}

	s.Builtin = false
	s.Type = "" // agent skill
	s.Order = m.nextOrder
	m.nextOrder++
	m.skills[s.Name] = &s
	return m.saveSkill(&s)
}

// InstallMCP 安装 MCP 技能
func (m *Manager) InstallMCP(ctx context.Context, s Skill) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.skills[s.Name]; ok && existing.Builtin {
		return fmt.Errorf("不能覆盖内置技能: %s", s.Name)
	}

	s.Builtin = false
	s.Type = "mcp"
	s.Enabled = true
	s.Order = m.nextOrder
	m.nextOrder++
	m.skills[s.Name] = &s

	// 连接 MCP 服务器并获取工具列表
	if s.MCP != nil {
		if err := m.connectMCP(ctx, &s); err != nil {
			delete(m.skills, s.Name)
			return fmt.Errorf("MCP 连接失败: %w", err)
		}
		// 更新技能（connectMCP 会填充 Tools）
		m.skills[s.Name] = &s
	}

	return m.saveSkill(&s)
}

// Uninstall 卸载技能
func (m *Manager) Uninstall(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.skills[name]
	if !ok {
		return fmt.Errorf("技能不存在: %s", name)
	}
	if s.Builtin {
		return fmt.Errorf("不能卸载内置技能: %s", name)
	}

	// 断开 MCP 连接
	if s.Type == "mcp" {
		m.disconnectMCP(name)
	}

	delete(m.skills, name)
	return m.deleteSkillDir(name)
}

// SetEnabled 启用/禁用技能
func (m *Manager) SetEnabled(name string, enabled bool) error {
	return m.SetEnabledWithCtx(context.Background(), name, enabled)
}

// SetEnabledWithCtx 启用/禁用技能（带 context，用于 MCP 重连）
func (m *Manager) SetEnabledWithCtx(ctx context.Context, name string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.skills[name]
	if !ok {
		return fmt.Errorf("技能不存在: %s", name)
	}
	s.Enabled = enabled

	// MCP 技能启用/禁用时管理连接
	if s.Type == "mcp" && s.MCP != nil {
		if enabled {
			if _, exists := m.mcpClients[name]; !exists {
				if err := m.connectMCP(ctx, s); err != nil {
					s.Enabled = false
					return fmt.Errorf("MCP 连接失败: %w", err)
				}
			}
		} else {
			m.disconnectMCP(name)
		}
	}

	return m.saveSkill(s)
}

// GetSkillSummary 生成已安装技能的摘要（不含 SKILL.md 正文，正文按需加载）
func (m *Manager) GetSkillSummary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var skills []string

	// 列出延迟加载的内置技能（如浏览器工具）
	for _, s := range m.skills {
		if !s.Enabled || !s.Deferred {
			continue
		}
		displayName := s.DisplayName
		if displayName == "" {
			displayName = s.Name
		}
		skills = append(skills, fmt.Sprintf("- 【%s】 - %s（需要时先调用 %s 激活）", displayName, s.Description, s.Tools[0].Name))
	}

	// 列出内置系统工具
	for _, s := range m.skills {
		if !s.Enabled || !s.Builtin || s.Deferred {
			continue
		}
		displayName := s.DisplayName
		if displayName == "" {
			displayName = s.Name
		}
		var toolNames []string
		for _, t := range s.Tools {
			toolNames = append(toolNames, t.Name)
		}
		skills = append(skills, fmt.Sprintf("- 【%s】 - %s（工具：%s）", displayName, s.Description, strings.Join(toolNames, "、")))
	}

	// 列出 SKILL.md 技能
	for _, s := range m.skills {
		if !s.Enabled || s.Format != "skillmd" {
			continue
		}
		displayName := s.DisplayName
		if displayName == "" {
			displayName = s.Name
		}
		skills = append(skills, fmt.Sprintf("- 【%s】 - %s（调用此工具获取使用说明，再用 execute_command 执行）", displayName, s.Description))
	}
	// 列出 MCP 技能
	for _, s := range m.skills {
		if !s.Enabled || s.Type != "mcp" {
			continue
		}
		displayName := s.DisplayName
		if displayName == "" {
			displayName = s.Name
		}
		var toolNames []string
		for _, t := range s.Tools {
			toolNames = append(toolNames, t.Name)
		}
		skills = append(skills, fmt.Sprintf("- 【%s】 - %s（MCP 工具：%s）", displayName, s.Description, strings.Join(toolNames, "、")))
	}

	// 列出 yaml 格式技能
	for _, s := range m.skills {
		if !s.Enabled || s.Format != "yaml" {
			continue
		}
		displayName := s.DisplayName
		if displayName == "" {
			displayName = s.Name
		}
		var toolNames []string
		for _, t := range s.Tools {
			toolNames = append(toolNames, t.Name)
		}
		skills = append(skills, fmt.Sprintf("- 【%s】 - %s（工具：%s）", displayName, s.Description, strings.Join(toolNames, "、")))
	}

	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== 已安装技能（%d 个）===\n", len(skills))
	for _, s := range skills {
		sb.WriteString(s + "\n")
	}
	return sb.String()
}

// GetSkillContent 按需获取 SKILL.md 技能的完整内容（用于工具调用时注入）
func (m *Manager) GetSkillContent(toolName string) string {
	// 实时从文件读取 SKILL.md 内容
	mdPath := filepath.Join(skillsDir(), toolName, "SKILL.md")
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return ""
	}
	_, body := splitFrontmatter(string(data))
	if body == "" {
		return ""
	}
	return fmt.Sprintf("=== %s SKILL.md 使用说明 ===\n%s\n=== END ===", toolName, body)
}

// GetToolDefinitions 获取所有已启用技能的工具定义（OpenAI function calling 格式）
func (m *Manager) GetToolDefinitions() []map[string]any {
	return m.GetToolDefinitionsFiltered(nil)
}

// GetToolDefinitionsFiltered 获取工具定义，支持延迟加载过滤
// activated: 已激活的延迟技能名集合，nil 表示包含所有工具
func (m *Manager) GetToolDefinitionsFiltered(activated map[string]bool) []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []map[string]any
	for _, s := range m.skills {
		if !s.Enabled {
			continue
		}
		// 延迟加载技能未激活时，只发送第一个工具（激活工具）
		skillTools := s.Tools
		if s.Deferred && activated != nil && !activated[s.Name] {
			if len(skillTools) > 0 {
				skillTools = skillTools[:1]
			}
		}
		for _, t := range skillTools {
			props := make(map[string]any)
			for k, v := range t.Parameters.Properties {
				prop := map[string]any{
					"type":        v.Type,
					"description": v.Description,
				}
				if v.Type == "array" && v.Items != nil {
					prop["items"] = v.Items
				} else if v.Type == "array" {
					prop["items"] = map[string]any{"type": "string"} // 默认 string 数组
				}
				if len(v.Enum) > 0 {
					prop["enum"] = v.Enum
				}
				props[k] = prop
			}
			required := t.Parameters.Required
			if required == nil {
				required = []string{}
			}
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters": map[string]any{
						"type":       t.Parameters.Type,
						"properties": props,
						"required":   required,
					},
				},
			})
		}
	}
	return tools
}

// ToolResult 工具执行结果
type ToolResult struct {
	Output  string // 输出内容（返回给 LLM）
	Success bool   // 是否成功
}

// ExecuteTool 执行工具，返回结构化结果
func (m *Manager) ExecuteTool(toolName string, argsJSON string) ToolResult {
	// 先在读锁下查找工具，找到后释放锁再执行（避免 executor 回调 Manager 方法时死锁）
	m.mu.RLock()
	var foundTool *Tool
	var foundSkillName string
	for _, s := range m.skills {
		if !s.Enabled {
			continue
		}
		for i := range s.Tools {
			if s.Tools[i].Name == toolName {
				t := s.Tools[i]
				foundTool = &t
				foundSkillName = s.Name
				break
			}
		}
		if foundTool != nil {
			break
		}
	}
	m.mu.RUnlock()

	if foundTool == nil {
		return ToolResult{Output: fmt.Sprintf("未找到工具: %s", toolName), Success: false}
	}
	return m.executeToolImpl(*foundTool, argsJSON, foundSkillName)
}

// executeToolImpl 实际执行工具
func (m *Manager) executeToolImpl(t Tool, argsJSON string, skillName string) ToolResult {
	switch t.Execute.Type {
	case "builtin":
		return m.executeBuiltin(t.Name, argsJSON)
	case "command":
		return executeCommand(t.Execute.Command, argsJSON)
	case "mcp":
		return m.executeMCP(skillName, t.Name, argsJSON)
	default:
		return ToolResult{Output: fmt.Sprintf("不支持的执行类型: %s", t.Execute.Type), Success: false}
	}
}

// executeBuiltin 执行内置工具
func (m *Manager) executeBuiltin(name string, argsJSON string) ToolResult {
	fn, ok := m.builtinExecutors[name]
	if !ok {
		return ToolResult{Output: fmt.Sprintf("未注册的内置工具: %s", name), Success: false}
	}
	var args map[string]any
	if err := parseJSON(argsJSON, &args); err != nil {
		return ToolResult{Output: fmt.Sprintf("参数解析失败: %v", err), Success: false}
	}
	return fn(args)
}

// ===== SKILL.md 解析 =====

// parseSkillMD 从 SKILL.md 内容解析出 Skill（frontmatter + 正文）
func parseSkillMD(content string, dirName string) *Skill {
	fm, body := splitFrontmatter(content)

	name := fm["name"]
	if name == "" {
		name = strings.ToLower(dirName)
	}
	desc := fm["description"]

	s := &Skill{
		Name:        name,
		DisplayName: name,
		Description: desc,
		Version:     "1.0.0",
		Enabled:     true,
		Format:      "skillmd",
		Content:     body,
		// 注册同名工具：调用即加载 SKILL.md 说明，LLM 再用 execute_command 执行
		Tools: []Tool{
			{
				Name:        name,
				Description: desc,
				Parameters:  ToolParam{Type: "object", Properties: map[string]PropDef{}},
				Execute:     ToolExecute{Type: "builtin"},
			},
		},
	}
	return s
}

// splitFrontmatter 分离 YAML frontmatter 和正文
func splitFrontmatter(content string) (map[string]string, string) {
	fm := make(map[string]string)
	if !strings.HasPrefix(content, "---") {
		return fm, content
	}
	rest := content[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return fm, content
	}
	fmText := rest[:end]
	body := strings.TrimSpace(rest[end+4:])

	for _, line := range strings.Split(fmText, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, "\"'")
			fm[key] = val
		}
	}
	return fm, body
}

// ===== MCP 管理 =====

// connectMCP 连接 MCP 服务器并加载工具列表（调用方需持有 mu 锁）
func (m *Manager) connectMCP(ctx context.Context, s *Skill) error {
	if s.MCP == nil {
		return fmt.Errorf("MCP 配置为空")
	}

	client := NewMCPClient(*s.MCP)
	if err := client.Connect(ctx); err != nil {
		return err
	}

	// 获取工具列表
	mcpTools, err := client.ListTools()
	if err != nil {
		client.Close()
		return fmt.Errorf("获取 MCP 工具列表失败: %w", err)
	}

	// 转换 MCP 工具为内部 Tool 格式
	s.Tools = make([]Tool, 0, len(mcpTools))
	for _, mt := range mcpTools {
		tool := Tool{
			Name:        mt.Name,
			Description: mt.Description,
			Execute:     ToolExecute{Type: "mcp"},
		}
		// 从 MCP inputSchema 提取参数定义
		tool.Parameters = mcpSchemaToToolParam(mt.InputSchema)
		s.Tools = append(s.Tools, tool)
	}

	m.mcpClients[s.Name] = client
	return nil
}

// disconnectMCP 断开 MCP 连接（调用方需持有 mu 锁）
func (m *Manager) disconnectMCP(name string) {
	if client, ok := m.mcpClients[name]; ok {
		client.Close()
		delete(m.mcpClients, name)
	}
}

// executeMCP 通过 MCP 客户端执行工具
func (m *Manager) executeMCP(skillName, toolName, argsJSON string) ToolResult {
	client, ok := m.mcpClients[skillName]
	if !ok {
		return ToolResult{Output: fmt.Sprintf("MCP 服务器未连接: %s", skillName), Success: false}
	}

	var args map[string]any
	if err := parseJSON(argsJSON, &args); err != nil {
		return ToolResult{Output: fmt.Sprintf("参数解析失败: %v", err), Success: false}
	}

	result, err := client.CallTool(toolName, args)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("MCP 工具调用失败: %v", err), Success: false}
	}
	return ToolResult{Output: result, Success: true}
}

// Shutdown 关闭所有 MCP 连接
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.mcpClients {
		client.Close()
		delete(m.mcpClients, name)
	}
}

// mcpSchemaToToolParam 将 MCP 的 inputSchema 转换为 ToolParam
func mcpSchemaToToolParam(schema map[string]any) ToolParam {
	tp := ToolParam{Type: "object"}
	tp.Properties = make(map[string]PropDef)

	if props, ok := schema["properties"].(map[string]any); ok {
		for k, v := range props {
			if propMap, ok := v.(map[string]any); ok {
				pd := PropDef{}
				if t, ok := propMap["type"].(string); ok {
					pd.Type = t
				}
				if d, ok := propMap["description"].(string); ok {
					pd.Description = d
				}
				// 保留 array 的 items 定义
				if items, ok := propMap["items"].(map[string]any); ok {
					pd.Items = items
				}
				// 保留 enum 定义
				if enumArr, ok := propMap["enum"].([]any); ok {
					for _, e := range enumArr {
						if s, ok := e.(string); ok {
							pd.Enum = append(pd.Enum, s)
						}
					}
				}
				tp.Properties[k] = pd
			}
		}
	}

	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				tp.Required = append(tp.Required, s)
			}
		}
	}

	return tp
}
