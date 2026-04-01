package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"clawdesk/src/config"
	"clawdesk/src/memory"
)

// Session 助手/机器人
type Session struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Avatar       string    `json:"avatar"`
	Description  string    `json:"description"`
	SystemPrompt string    `json:"systemPrompt"`
	ProviderID   string    `json:"providerId"`
	Model        string    `json:"model"`
	CreatedAt    time.Time `json:"createdAt"`
	History      []Message `json:"history"`
}

// BotOptions 创建/更新助手的参数
type BotOptions struct {
	Name         string `json:"name"`
	Avatar       string `json:"avatar"`
	Description  string `json:"description"`
	SystemPrompt string `json:"systemPrompt"`
	ProviderID   string `json:"providerId"`
	Model        string `json:"model"`
}

// SessionManager 会话管理器
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	order    []string
	memMgr   *memory.MemoryManager
}

func NewSessionManager(memMgr *memory.MemoryManager) *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
		memMgr:   memMgr,
	}
	sm.load()
	return sm
}

func (sm *SessionManager) load() {
	metas, err := sm.memMgr.Store.ListSessions()
	if err != nil {
		return
	}
	for _, meta := range metas {
		// 迁移：清除旧版硬编码的默认提示词，改为空（运行时动态生成）
		if strings.HasPrefix(meta.SystemPrompt, "你是一个AI助手，可以通过工具帮用户完成实际操作") {
			meta.SystemPrompt = ""
			sm.memMgr.Store.SaveMeta(&meta)
		}
		sm.sessions[meta.ID] = metaToSession(&meta)
		sm.order = append(sm.order, meta.ID)
	}
}

func metaToSession(meta *memory.SessionMeta) *Session {
	return &Session{
		ID:           meta.ID,
		Name:         meta.Name,
		Avatar:       meta.Avatar,
		Description:  meta.Description,
		SystemPrompt: meta.SystemPrompt,
		ProviderID:   meta.ProviderID,
		Model:        meta.Model,
		CreatedAt:    meta.CreatedAt,
	}
}

func sessionToMeta(s *Session) *memory.SessionMeta {
	return &memory.SessionMeta{
		ID:           s.ID,
		Name:         s.Name,
		Avatar:       s.Avatar,
		Description:  s.Description,
		SystemPrompt: s.SystemPrompt,
		ProviderID:   s.ProviderID,
		Model:        s.Model,
		CreatedAt:    s.CreatedAt,
	}
}

// List 获取所有助手（不含历史）
func (sm *SessionManager) List() []Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]Session, 0, len(sm.order))
	for _, id := range sm.order {
		if s, ok := sm.sessions[id]; ok {
			result = append(result, Session{
				ID:          s.ID,
				Name:        s.Name,
				Avatar:      s.Avatar,
				Description: s.Description,
				ProviderID:  s.ProviderID,
				Model:       s.Model,
				CreatedAt:   s.CreatedAt,
			})
		}
	}
	return result
}

// CreateBot 创建新助手
func (sm *SessionManager) CreateBot(opts BotOptions) Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id := generateID()
	avatar := opts.Avatar
	if avatar == "" {
		avatar = "🤖"
	}
	name := opts.Name
	if name == "" {
		name = generateBotName()
	}

	s := &Session{
		ID:           id,
		Name:         name,
		Avatar:       avatar,
		Description:  opts.Description,
		SystemPrompt: opts.SystemPrompt, // 空表示使用默认，仅存用户自定义内容
		ProviderID:   opts.ProviderID,
		Model:        opts.Model,
		CreatedAt:    time.Now(),
	}

	sm.sessions[id] = s
	sm.order = append(sm.order, id)
	sm.memMgr.Store.SaveMeta(sessionToMeta(s))

	return *s
}

// UpdateBot 更新助手属性
func (sm *SessionManager) UpdateBot(id string, opts BotOptions) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.sessions[id]
	if !ok {
		return
	}

	if opts.Name != "" {
		s.Name = opts.Name
	}
	if opts.Avatar != "" {
		s.Avatar = opts.Avatar
	}
	s.Description = opts.Description
	if opts.SystemPrompt != "" {
		s.SystemPrompt = opts.SystemPrompt
	}
	s.ProviderID = opts.ProviderID
	s.Model = opts.Model

	sm.memMgr.Store.SaveMeta(sessionToMeta(s))
}

// Reorder 重新排序助手
func (sm *SessionManager) Reorder(ids []string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 校验：新顺序必须包含所有现有 ID
	existing := make(map[string]bool, len(sm.order))
	for _, id := range sm.order {
		existing[id] = true
	}
	var newOrder []string
	seen := make(map[string]bool)
	for _, id := range ids {
		if existing[id] && !seen[id] {
			newOrder = append(newOrder, id)
			seen[id] = true
		}
	}
	// 补上未出现的（防丢失）
	for _, id := range sm.order {
		if !seen[id] {
			newOrder = append(newOrder, id)
		}
	}
	sm.order = newOrder
}

// Delete 删除助手
func (sm *SessionManager) Delete(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.sessions, id)
	for i, oid := range sm.order {
		if oid == id {
			sm.order = append(sm.order[:i], sm.order[i+1:]...)
			break
		}
	}
	sm.memMgr.DeleteSession(id)
}

// Get 获取助手
func (sm *SessionManager) Get(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if s, ok := sm.sessions[id]; ok {
		return s
	}
	return nil
}

// AppendMessage 追加消息
func (sm *SessionManager) AppendMessage(id string, msg Message) {
	sm.mu.RLock()
	_, ok := sm.sessions[id]
	sm.mu.RUnlock()
	if !ok {
		return
	}
	stored := memory.StoredMessage{
		Timestamp: time.Now(),
		Role:      msg.Role,
		Content:   msg.GetContentText(),
	}
	sm.memMgr.StoreMessage(id, stored)
}

// GetRecentHistory 获取最近历史
func (sm *SessionManager) GetRecentHistory(id string) []Message {
	msgs, err := sm.memMgr.Store.LoadRecentMessages(id, 100)
	if err != nil {
		return []Message{}
	}
	var result []Message
	for _, m := range msgs {
		result = append(result, Message{
			Role:      m.Role,
			Content:   m.Content,
			Timestamp: m.Timestamp.Format("15:04"),
		})
	}
	return result
}

// ClearHistory 清空历史
func (sm *SessionManager) ClearHistory(id string) {
	sm.memMgr.DeleteSession(id)
	if s, ok := sm.sessions[id]; ok {
		sm.memMgr.Store.SaveMeta(sessionToMeta(s))
	}
}

func generateID() string {
	return time.Now().Format("20060102150405") + randomSuffix()
}

func randomSuffix() string {
	b := make([]byte, 4)
	n := time.Now().UnixNano()
	for i := range b {
		b[i] = byte('a' + (n>>(i*8))%26)
	}
	return string(b)
}

// generateBotName 生成一个随机的英文助手名称
func generateBotName() string {
	adjectives := []string{
		"Swift", "Bright", "Clever", "Noble", "Vivid",
		"Sharp", "Calm", "Bold", "Lucky", "Wise",
		"Gentle", "Keen", "Nimble", "Brave", "Witty",
	}
	nouns := []string{
		"Atlas", "Nova", "Echo", "Spark", "Pixel",
		"Sage", "Orbit", "Flux", "Prism", "Cipher",
		"Nexus", "Pulse", "Coda", "Lumen", "Apex",
	}
	n := time.Now().UnixNano()
	adj := adjectives[n%int64(len(adjectives))]
	noun := nouns[(n/17)%int64(len(nouns))]
	return adj + " " + noun
}

// GetEffectivePrompt 获取会话的有效系统提示词（自定义 > 默认）
func GetEffectivePrompt(session *Session) string {
	if session.SystemPrompt != "" {
		return session.SystemPrompt
	}
	return defaultSystemPrompt(session.ID)
}

// workspaceDir 获取会话的工作区目录
func workspaceDir(sessionID string) string {
	return filepath.Join(config.GetConfigDir(), "sessions", sessionID, "workspace")
}

func defaultSystemPrompt(sessionID string) string {
	workspace := workspaceDir(sessionID)
	os.MkdirAll(workspace, 0755)

	// 系统提示词中不暴露绝对路径，LLM 只使用相对路径，后端自动解析到 workspace
	_ = workspace
	return `你是一个AI助手，可以通过工具帮用户完成实际操作。

你的可用工具由已安装的技能（Skill）提供，每个技能包含一组工具。当用户问"你有哪些技能/skill"时，请按技能分组回答，不要把单个工具当成独立技能。

重要规则：
1. 当用户要求执行操作时，必须立即调用工具，不能只回复文字。不要询问用户补充信息，用默认值直接执行
2. 先执行工具，根据工具的实际返回结果告诉用户。如果用户只是聊天问答，正常回复即可
3. 文件路径一律使用相对路径（如 myproject/src/App.js），系统会自动解析到工作区目录
4. 使用已安装技能时，先调用技能同名工具获取使用说明，再根据说明用 execute_command 执行命令
5. 当用户请求涉及多个独立任务时（如"查天气同时看GitHub仓库"），调用 plan_and_execute 工具并行执行
6. 使用 fetch_url 时优先请求 JSON API，失败时自动尝试替代来源（最多 3 个）

前端项目开发流程（必须一口气执行完所有步骤，中途不要停下来等用户确认）：
1. 项目初始化：execute_command 创建项目（如 npm create vite@latest myproject -- --template react），项目名由你根据任务内容决定
2. 安装依赖：execute_command 在项目目录下执行 npm install(workDir 设为项目目录名),依赖安装成功后，才可以继续进行，如果安装出错，直接提示用户
3. 配置自动打开浏览器：用 write_file 修改 vite.config.js（或 vite.config.ts），在 defineConfig 中添加 server: { open: true }，这样 dev server 启动后浏览器会自动打开
4. 编写功能代码【最关键】：用 write_file 覆写 src/App.jsx、src/App.css 等文件，写入完整的业务功能代码。脚手架只生成空壳，这一步不做等于没做
5. 启动服务：用 open_terminal 工具在系统终端中启动dev server: 需要从package.json中获取真正的启动命令(如start,dev等)
以上 5 步必须连续执行，全部完成后再回复用户
6. 当用户需要修改已有项目时：先用 list_directory 查看工作区确认项目目录名，再用 read_file 读取要修改的文件，最后用 write_file 写入修改后的完整文件。不要创建新项目
7. 如果 dev server 已在终端中运行，不要重复启动。修改代码后 Vite 会自动热更新，告诉用户查看浏览器即可`
}
