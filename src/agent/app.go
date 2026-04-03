package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"clawdesk/src/audit"
	"clawdesk/src/channels"
	"clawdesk/src/config"
	"clawdesk/src/memory"
	"clawdesk/src/skill"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 主应用结构
type App struct {
	ctx          context.Context
	sessionMgr   *SessionManager
	skillMgr     *skill.Manager
	memMgr       *memory.MemoryManager
	usageTracker *UsageTracker
	auditDB      *audit.DB
	scheduler        *Scheduler
	channelMgr       *channels.Manager
	activeSessionID  string // 当前正在处理的会话 ID（供 create_schedule 使用）
	sessionCancels   map[string]context.CancelFunc // 每个会话独立的取消函数
	sessionMu        sync.Mutex                    // 保护 sessionCancels
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// sanitizePath 将文本中的 workspace 绝对路径替换为相对路径
func sanitizePath(sessionID, text string) string {
	if sessionID == "" || text == "" {
		return text
	}
	ws := workspaceDir(sessionID)
	text = strings.ReplaceAll(text, ws+"/", "")
	text = strings.ReplaceAll(text, ws, ".")
	return text
}

// Shutdown 应用退出时释放资源
func (a *App) Shutdown(ctx context.Context) {
	if a.scheduler != nil {
		a.scheduler.Shutdown()
	}
	if a.channelMgr != nil {
		a.channelMgr.Shutdown()
	}
	skill.ShutdownBrowser()
	if a.skillMgr != nil {
		a.skillMgr.Shutdown()
	}
	if a.memMgr != nil {
		a.memMgr.Close()
	}
	if a.auditDB != nil {
		a.auditDB.Close()
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.sessionCancels = make(map[string]context.CancelFunc)

	// 创建 LLM 调用闭包（注入给 SkillManager 和 MemoryManager）
	llmCall := func(callCtx context.Context, systemPrompt, userPrompt string) (string, error) {
		msgs := []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
		return DoNonStreamRequest(callCtx, msgs, nil)
	}

	// 创建带工具调用的 LLM 闭包（注入给 Skill Vetter 安全审查）
	vetLLMCall := func(callCtx context.Context, vetMsgs []skill.VetMessage, tools []map[string]any) (*skill.VetLLMResponse, error) {
		// 将 VetMessage 转换为 Message
		msgs := make([]Message, len(vetMsgs))
		for i, vm := range vetMsgs {
			msgs[i] = Message{
				Role:       vm.Role,
				Content:    vm.Content,
				ToolCallID: vm.ToolCallID,
				Name:       vm.Name,
			}
			// 转换 tool_calls
			if len(vm.ToolCalls) > 0 {
				msgs[i].ToolCalls = make([]ToolCall, len(vm.ToolCalls))
				for j, tc := range vm.ToolCalls {
					msgs[i].ToolCalls[j] = ToolCall{
						ID:   tc.ID,
						Type: "function",
						Function: FunctionCall{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					}
				}
			}
		}
		resp, err := DoNonStreamRequestWithTools(callCtx, msgs, tools)
		if err != nil {
			return nil, err
		}
		// 转换响应
		vetResp := &skill.VetLLMResponse{Content: resp.Content}
		for _, tc := range resp.ToolCalls {
			vetResp.ToolCalls = append(vetResp.ToolCalls, skill.VetToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		return vetResp, nil
	}
	a.skillMgr = skill.NewManager(ctx, llmCall, vetLLMCall)

	// 初始化记忆管理器
	memMgr, err := memory.NewMemoryManager(config.GetConfigDir(), llmCall)
	if err != nil {
		println("初始化记忆系统失败:", err.Error())
		return
	}
	a.memMgr = memMgr

	// 后台下载嵌入模型资源（首次启动时下载，后续跳过）
	memMgr.StartAssetDownload(func(fileName string, current, total int64) {
		runtime.EventsEmit(ctx, "embedding:download", map[string]any{
			"file":    fileName,
			"current": current,
			"total":   total,
		})
	})

	a.sessionMgr = NewSessionManager(memMgr)
	a.usageTracker = NewUsageTracker()

	auditDB, err := audit.NewDB(config.GetConfigDir())
	// 注入存储审计到 MemoryManager
	if auditDB != nil {
		memMgr.Auditor = auditDB
	}
	if err != nil {
		println("初始化审计数据库失败:", err.Error())
	}
	a.auditDB = auditDB

	// 启动系统资源监控
	a.StartSystemMonitor()

	// 启动定时任务调度器
	a.scheduler = NewScheduler(a)

	// 初始化渠道管理器，自动连接所有已启用渠道
	a.channelMgr = channels.NewManager(func(channelID, userID, text string) string {
		return a.handleChannelMessage(channelID, userID, text)
	})
	a.channelMgr.ConnectAll()

	// 定时刷新技能列表（每 30 秒扫描 skills 目录）
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				a.skillMgr.ReloadCustomSkills(a.ctx)
			}
		}
	}()

	// 设置 workspace 解析器：内置工具的路径/工作目录默认解析到当前会话的 workspace
	a.skillMgr.SetWorkspaceResolver(func() string {
		if a.activeSessionID != "" {
			ws := workspaceDir(a.activeSessionID)
			os.MkdirAll(ws, 0755)
			return ws
		}
		return ""
	})

	// 注册 create_bot 工具执行器（需要访问 sessionMgr）
	a.skillMgr.RegisterBuiltinExecutor("create_bot", func(args map[string]any) skill.ToolResult {
		name, _ := args["name"].(string)
		avatar, _ := args["avatar"].(string)
		desc, _ := args["description"].(string)
		prompt, _ := args["systemPrompt"].(string)

		bot := a.sessionMgr.CreateBot(BotOptions{
			Name:         name,
			Avatar:       avatar,
			Description:  desc,
			SystemPrompt: prompt,
		})

		// 通知前端刷新助手列表
		runtime.EventsEmit(a.ctx, "bot:created", bot.ID, bot.Name)

		return skill.ToolResult{Output: fmt.Sprintf("助手已创建成功: %s %s (ID: %s)", bot.Avatar, bot.Name, bot.ID), Success: true}
	})

	// 注册 manage_schedule 执行器（管理定时任务：查看/创建/停止/启用/删除）
	a.skillMgr.RegisterBuiltinExecutor("manage_schedule", func(args map[string]any) skill.ToolResult {
		action, _ := args["action"].(string)

		switch action {
		case "list":
			tasks := a.scheduler.ListTasks(a.activeSessionID)
			if len(tasks) == 0 {
				return skill.ToolResult{Output: "当前助手没有定时任务", Success: true}
			}
			var sb strings.Builder
			for _, t := range tasks {
				status := "已启用"
				if !t.Enabled {
					status = "已停止"
				}
				sched := ""
				if t.Schedule.Type == "interval" {
					sched = fmt.Sprintf("每 %d 分钟", t.Schedule.Interval)
				} else {
					sched = fmt.Sprintf("每天 %s", t.Schedule.DailyAt)
				}
				fmt.Fprintf(&sb, "- ID: %s | 名称: %s | %s | %s | 已执行 %d 次\n", t.ID, t.Name, sched, status, t.RunCount)
			}
			return skill.ToolResult{Output: sb.String(), Success: true}

		case "create":
			name, _ := args["name"].(string)
			prompt, _ := args["prompt"].(string)
			if name == "" || prompt == "" {
				return skill.ToolResult{Output: "错误: create 操作需要 name 和 prompt", Success: false}
			}
			schedType, _ := args["scheduleType"].(string)
			if schedType == "" {
				schedType = "interval"
			}
			interval := 30
			if v, ok := args["interval"].(float64); ok {
				interval = int(v)
			}
			dailyAt, _ := args["dailyAt"].(string)
			repeatType, _ := args["repeatType"].(string)
			if repeatType == "" {
				repeatType = "forever"
			}
			repeatCount := 0
			if v, ok := args["repeatCount"].(float64); ok {
				repeatCount = int(v)
			}
			task := &ScheduledTask{
				SessionID: a.activeSessionID,
				Name:      name,
				Prompt:    prompt,
				Enabled:   true,
				Schedule:  ScheduleConfig{Type: schedType, Interval: interval, DailyAt: dailyAt, RepeatType: repeatType, RepeatCount: repeatCount},
			}
			a.scheduler.AddTask(task)
			desc := fmt.Sprintf("定时任务已创建: %s (ID: %s)", name, task.ID)
			if schedType == "interval" {
				desc += fmt.Sprintf("，每 %d 分钟执行一次", interval)
			} else if dailyAt != "" {
				desc += fmt.Sprintf("，每天 %s 执行", dailyAt)
			}
			return skill.ToolResult{Output: desc, Success: true}

		case "stop":
			taskID, _ := args["taskId"].(string)
			if taskID == "" {
				// 没指定 ID，尝试按名称匹配
				return skill.ToolResult{Output: "请先用 list 查看任务列表获取 taskId，再执行 stop", Success: false}
			}
			a.scheduler.SetTaskEnabled(taskID, false)
			return skill.ToolResult{Output: fmt.Sprintf("定时任务已停止: %s", taskID), Success: true}

		case "enable":
			taskID, _ := args["taskId"].(string)
			if taskID == "" {
				return skill.ToolResult{Output: "请提供 taskId", Success: false}
			}
			a.scheduler.SetTaskEnabled(taskID, true)
			return skill.ToolResult{Output: fmt.Sprintf("定时任务已启用: %s", taskID), Success: true}

		case "delete":
			taskID, _ := args["taskId"].(string)
			if taskID == "" {
				return skill.ToolResult{Output: "请提供 taskId", Success: false}
			}
			a.scheduler.DeleteTask(taskID)
			return skill.ToolResult{Output: fmt.Sprintf("定时任务已删除: %s", taskID), Success: true}

		default:
			return skill.ToolResult{Output: "未知操作: " + action + "。支持: list/create/stop/enable/delete", Success: false}
		}
	})

	// 注册 manage_skill 执行器（搜索、安装、卸载、查看技能）
	a.skillMgr.RegisterBuiltinExecutor("manage_skill", func(args map[string]any) skill.ToolResult {
		action, _ := args["action"].(string)

		switch action {
		case "list":
			skills := a.skillMgr.List()
			var custom []skill.Skill
			for _, s := range skills {
				if !s.Builtin {
					custom = append(custom, s)
				}
			}
			if len(custom) == 0 {
				return skill.ToolResult{Output: "当前没有已安装的自定义技能。", Success: true}
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("已安装 %d 个自定义技能:\n", len(custom)))
			for _, s := range custom {
				level := ""
				if s.SecurityLevel != "" {
					level = fmt.Sprintf(" [%s]", s.SecurityLevel)
				}
				sb.WriteString(fmt.Sprintf("- %s (v%s)%s - %s\n", s.DisplayName, s.Version, level, s.Description))
			}
			return skill.ToolResult{Output: sb.String(), Success: true}

		case "search":
			query, _ := args["query"].(string)
			if query == "" {
				return skill.ToolResult{Output: "错误: query 参数为空", Success: false}
			}
			results, err := skill.SearchClawHub(query)
			if err != nil {
				return skill.ToolResult{Output: fmt.Sprintf("搜索失败: %v", err), Success: false}
			}
			if len(results) == 0 {
				return skill.ToolResult{Output: "未找到匹配的技能", Success: true}
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("找到 %d 个技能:\n", len(results)))
			for i, r := range results {
				sb.WriteString(fmt.Sprintf("%d. %s - %s (href: %s)\n", i+1, r.Name, r.Desc, r.Href))
			}
			sb.WriteString("\n用 install 操作并传入 href 来安装技能。")
			return skill.ToolResult{Output: sb.String(), Success: true}

		case "install":
			href, _ := args["href"].(string)
			if href == "" {
				return skill.ToolResult{Output: "错误: href 参数为空，请先 search 获取技能的 href", Success: false}
			}
			parts := strings.Split(strings.TrimPrefix(href, "/"), "/")
			slug := parts[len(parts)-1]
			if existing := a.skillMgr.Get(slug); existing != nil {
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("技能 %s 已安装，无需重复安装。\n\n", existing.DisplayName))
				sb.WriteString(fmt.Sprintf("版本: %s\n描述: %s\n", existing.Version, existing.Description))
				if existing.SecurityLevel != "" {
					sb.WriteString(fmt.Sprintf("安全等级: %s\n", existing.SecurityLevel))
				}
				return skill.ToolResult{Output: sb.String(), Success: true}
			}
			skillName, err := skill.InstallClawHubSkill(href)
			if err != nil {
				return skill.ToolResult{Output: fmt.Sprintf("安装失败: %v", err), Success: false}
			}
			a.skillMgr.ReloadCustomSkills(a.ctx)

			s := a.skillMgr.Get(skillName)
			if s == nil {
				return skill.ToolResult{Output: fmt.Sprintf("技能 %s 未通过安全检查，已被拒绝安装。", skillName), Success: false}
			}

			runtime.EventsEmit(a.ctx, "skill:installed", skillName)

			var sb strings.Builder
			sb.WriteString("技能安装成功！\n\n")
			sb.WriteString(fmt.Sprintf("名称: %s\n版本: %s\n描述: %s\n工具数: %d\n状态: 已启用\n", s.DisplayName, s.Version, s.Description, len(s.Tools)))
			if s.SecurityLevel != "" {
				sb.WriteString(fmt.Sprintf("安全等级: %s\n", s.SecurityLevel))
				if s.SecurityNote != "" {
					sb.WriteString(fmt.Sprintf("安全备注: %s\n", s.SecurityNote))
				}
			}
			return skill.ToolResult{Output: sb.String(), Success: true}

		case "uninstall":
			name, _ := args["name"].(string)
			if name == "" {
				return skill.ToolResult{Output: "错误: name 参数为空", Success: false}
			}
			s := a.skillMgr.Get(name)
			if s == nil {
				return skill.ToolResult{Output: fmt.Sprintf("技能 %s 不存在", name), Success: false}
			}
			if s.Builtin {
				return skill.ToolResult{Output: fmt.Sprintf("内置技能 %s 不可卸载", name), Success: false}
			}
			if err := a.skillMgr.Uninstall(name); err != nil {
				return skill.ToolResult{Output: fmt.Sprintf("卸载失败: %v", err), Success: false}
			}
			runtime.EventsEmit(a.ctx, "skill:installed", name)
			return skill.ToolResult{Output: fmt.Sprintf("技能 %s 已卸载", s.DisplayName), Success: true}

		default:
			return skill.ToolResult{Output: "未知操作: " + action + "。支持: list/search/install/uninstall", Success: false}
		}
	})

	// 注册 plan_and_execute 执行器（LLM 判断需要多步骤时自行调用）
	a.skillMgr.RegisterBuiltinExecutor("plan_and_execute", func(args map[string]any) skill.ToolResult {
		summary, _ := args["summary"].(string)
		stepsJSON, _ := args["steps"].(string)

		if summary == "" || stepsJSON == "" {
			return skill.ToolResult{Output: "错误: summary 和 steps 参数必填", Success: false}
		}

		// 解析步骤
		var steps []planStep
		if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
			return skill.ToolResult{Output: fmt.Sprintf("steps JSON 解析失败: %v", err), Success: false}
		}

		// 构建并执行 Orchestrator
		orchSessionID := a.activeSessionID
		orch := NewOrchestrator(a.skillMgr, nil)
		orch.onToolCall = func(name, toolArgs string) {
			runtime.EventsEmit(a.ctx, LLMToolCall, orchSessionID, name, sanitizePath(orchSessionID, toolArgs))
		}
		orch.onToolResult = func(name, toolArgs, result string, success bool, dur int64) {
			if a.auditDB != nil {
				a.auditDB.RecordSkill(orchSessionID, "", a.skillMgr.GetSkillByTool(name), name, sanitizePath(orchSessionID, toolArgs), sanitizePath(orchSessionID, result), success, dur)
			}
		}

		plan, _, err := orch.ExecutePlan(context.Background(), a.buildCurrentMessages(), summary, steps)
		if err != nil {
			return skill.ToolResult{Output: "多步骤执行失败: " + err.Error(), Success: false}
		}

		// 存储追踪
		if plan != nil {
			messageTs := fmt.Sprintf("%d", time.Now().UnixMilli())
			trace := &ExecutionTrace{SessionID: a.activeSessionID, MessageTs: messageTs, Plan: plan}
			storeTrace(a.activeSessionID, messageTs, trace)
			runtime.EventsEmit(a.ctx, LLMTrace, a.activeSessionID, messageTs)
		}

		// 汇总各步骤结果
		var result strings.Builder
		for _, s := range plan.Steps {
			status := "✅"
			if s.Status == "failed" {
				status = "❌"
			}
			fmt.Fprintf(&result, "%s **%s** (%dms)\n%s\n\n", status, s.Name, s.Duration, s.Result)
		}
		return skill.ToolResult{Output: result.String(), Success: true}
	})
}

// buildCurrentMessages 构建当前会话的消息上下文（供 plan_and_execute 使用）
// generateSimpleMermaid 为普通工具调用生成简单的 mermaid 图
func generateSimpleMermaid(plan *TaskPlan) string {
	if len(plan.Steps) == 0 || len(plan.Steps[0].ToolCalls) == 0 {
		return ""
	}
	m := "graph LR\n"
	m += "    Q([用户提问]) --> LLM[LLM 思考]\n"
	for i, tc := range plan.Steps[0].ToolCalls {
		icon := "🔧"
		if !tc.Success {
			icon = "⚠️"
		}
		nodeID := fmt.Sprintf("T%d", i)
		m += fmt.Sprintf("    LLM --> %s(%s %s<br/>%dms)\n", nodeID, icon, tc.ToolName, tc.Duration)
	}
	m += "    LLM --> R([回复])\n"
	return m
}

// saveRequestLog 保存 LLM 请求日志（系统提示词 + 工具定义）
func (a *App) saveRequestLog(sessionID string, messages []Message) {
	var log memory.LLMRequestLog

	if len(messages) > 0 && messages[0].Role == "system" {
		if s, ok := messages[0].Content.(string); ok {
			log.SystemPrompt = s
		}
	}

	// 使用与 streamChat 一致的过滤逻辑（延迟技能仅暴露激活工具）
	log.Tools = a.skillMgr.GetToolDefinitionsFiltered(make(map[string]bool))

	a.memMgr.Store.SaveRequestLog(sessionID, log)
}

func (a *App) buildCurrentMessages() []Message {
	if a.activeSessionID == "" {
		return nil
	}
	session := a.sessionMgr.Get(a.activeSessionID)
	if session == nil {
		return nil
	}
	contextMsgs, err := a.memMgr.BuildContext(a.activeSessionID, GetEffectivePrompt(session), "")
	if err != nil {
		return nil
	}
	var messages []Message
	for _, m := range contextMsgs {
		messages = append(messages, Message{Role: m.Role, Content: m.Content})
	}
	if len(messages) > 0 && messages[0].Role == "system" {
		if s, ok := messages[0].Content.(string); ok {
			messages[0].Content = s + "\n\n" + a.skillMgr.GetSkillSummary()
		}
	}
	a.saveRequestLog(a.activeSessionID, messages)
	return messages
}

// ===== 会话/机器人管理 =====

func (a *App) GetSessions() []Session {
	return a.sessionMgr.List()
}

// ReorderSessions 重新排序助手
func (a *App) ReorderSessions(ids []string) {
	a.sessionMgr.Reorder(ids)
}

// CreateBot 创建新助手
func (a *App) CreateBot(opts BotOptions) Session {
	return a.sessionMgr.CreateBot(opts)
}

// UpdateBot 更新助手属性
func (a *App) UpdateBot(id string, opts BotOptions) {
	a.sessionMgr.UpdateBot(id, opts)
}

func (a *App) DeleteSession(id string) error {
	// 检查是否被渠道绑定
	cfg, _ := config.Load()
	if cfg != nil {
		for _, ch := range cfg.Channels {
			if ch.BotID == id {
				return fmt.Errorf("该助手已绑定渠道「%s」，不可单独删除", ch.Name)
			}
		}
	}
	a.sessionMgr.Delete(id)
	return nil
}

func (a *App) GetBotDetail(id string) *Session {
	return a.sessionMgr.Get(id)
}

func (a *App) GetSessionHistory(id string) []Message {
	return a.sessionMgr.GetRecentHistory(id)
}

func (a *App) ClearHistory(id string) {
	a.sessionMgr.ClearHistory(id)
}

func (a *App) GetRequestLogs(sessionID string) []memory.LLMRequestLog {
	records, _ := a.memMgr.Store.LoadRequestLogs(sessionID)
	return records
}

// ===== 聊天 =====

// SendMessage 发送消息到 LLM（流式返回，支持附件上传）
func (a *App) SendMessage(sessionID string, userInput string, attachments []Attachment) {
	a.activeSessionID = sessionID

	// 存储用户消息（纯文本部分）
	a.sessionMgr.AppendMessage(sessionID, Message{
		Role:    "user",
		Content: userInput,
	})

	// 通过记忆系统构建上下文
	session := a.sessionMgr.Get(sessionID)
	if session == nil {
		runtime.EventsEmit(a.ctx, LLMError, sessionID, "会话不存在")
		return
	}

	contextMsgs, err := a.memMgr.BuildContext(sessionID, GetEffectivePrompt(session), userInput)
	if err != nil {
		runtime.EventsEmit(a.ctx, LLMError, sessionID, "构建上下文失败: "+err.Error())
		return
	}

	// 转换为 LLM Message 格式
	var messages []Message
	for _, m := range contextMsgs {
		messages = append(messages, Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// 注入技能信息到系统提示词
	if len(messages) > 0 && messages[0].Role == "system" {
		if s, ok := messages[0].Content.(string); ok {
			messages[0].Content = s + "\n\n" + a.skillMgr.GetSkillSummary()
		}
	}
	// 构建用户消息（可能含附件）
	userMsg := buildUserMessage(userInput, attachments)
	messages = append(messages, userMsg)

	a.saveRequestLog(sessionID, messages)

	// 构建模型覆盖参数（助手绑定模型时使用）
	var llmOpts *LLMOptions
	if session.ProviderID != "" && session.Model != "" {
		llmOpts = &LLMOptions{ProviderID: session.ProviderID, Model: session.Model}
	}

	ctx, cancel := context.WithCancel(a.ctx)

	// 取消该会话之前的生成（如果有）
	a.sessionMu.Lock()
	if prev, ok := a.sessionCancels[sessionID]; ok {
		prev()
	}
	a.sessionCancels[sessionID] = cancel
	a.sessionMu.Unlock()

	go func() {
		defer func() {
			// panic 兜底：确保前端不会永远卡在 streaming 状态
			if r := recover(); r != nil {
				runtime.EventsEmit(a.ctx, LLMError, sessionID, fmt.Sprintf("内部错误: %v", r))
			}
			cancel()
			a.sessionMu.Lock()
			delete(a.sessionCancels, sessionID)
			a.sessionMu.Unlock()
		}()

		runtime.EventsEmit(a.ctx, LLMStart, sessionID)

		messageTs := fmt.Sprintf("%d", time.Now().UnixMilli())
		fullResponse := ""
		startTime := time.Now()

		// 收集工具调用记录
		var toolCalls []TaskToolCall

		usage, err := streamChat(ctx, messages, a.skillMgr, llmOpts,
			func(token string) {
				fullResponse += token
				runtime.EventsEmit(a.ctx, LLMToken, sessionID, token)
			},
			func(name, args string) {
				runtime.EventsEmit(a.ctx, LLMToolCall, sessionID, name, sanitizePath(sessionID, args))
				toolCalls = append(toolCalls, TaskToolCall{ToolName: name, Args: sanitizePath(sessionID, args)})
			},
			func(name, args, result string, success bool, dur int64) {
				runtime.EventsEmit(a.ctx, "llm:toolresult", sessionID, name, success)
				sArgs := sanitizePath(sessionID, args)
				sResult := sanitizePath(sessionID, result)
				if len(toolCalls) > 0 {
					last := &toolCalls[len(toolCalls)-1]
					last.Result = sResult
					last.Success = success
					last.Duration = dur
				}
				if a.auditDB != nil {
					go a.auditDB.RecordSkill(sessionID, session.Name, a.skillMgr.GetSkillByTool(name), name, sArgs, sResult, success, dur)
				}
			},
		)
		totalUsage := usage
		if err != nil {
			runtime.EventsEmit(a.ctx, LLMError, sessionID, err.Error())
		}

		endTime := time.Now()

		// 构建执行追踪（包含工具调用详情）
		summary := "直接回答"
		var steps []*TaskStep
		if len(toolCalls) > 0 {
			summary = fmt.Sprintf("调用了 %d 个工具", len(toolCalls))
			step := &TaskStep{
				ID: "1", Name: "工具调用", Description: userInput,
				Status: "done", ToolCalls: toolCalls,
				StartAt: startTime, EndAt: endTime,
				Duration: endTime.Sub(startTime).Milliseconds(),
			}
			steps = append(steps, step)
		}

		plan := &TaskPlan{
			Query: userInput, Summary: summary, Steps: steps,
			StartAt: startTime, EndAt: endTime,
			Duration: endTime.Sub(startTime).Milliseconds(),
		}
		plan.Mermaid = generateSimpleMermaid(plan)

		storeTrace(sessionID, messageTs, &ExecutionTrace{
			SessionID: sessionID, MessageTs: messageTs, Plan: plan,
		})
		runtime.EventsEmit(a.ctx, LLMTrace, sessionID, messageTs)

		// 记录 token 用量（优先用助手绑定的模型，否则用全局配置）
		usageProvider := session.ProviderID
		usageModel := session.Model
		if usageProvider == "" || usageModel == "" {
			if cfg, _ := config.Load(); cfg != nil {
				usageProvider = cfg.ActiveModel.ProviderID
				usageModel = config.GetActiveModelName(cfg)
			}
		}
		a.usageTracker.Record(usageProvider, usageModel, totalUsage)

		if fullResponse != "" {
			a.sessionMgr.AppendMessage(sessionID, Message{
				Role:    "assistant",
				Content: fullResponse,
			})
		}

		runtime.EventsEmit(a.ctx, LLMDone, sessionID)

		// 异步检查是否需要压缩
		a.memMgr.TriggerCompressionIfNeeded(a.ctx, sessionID)
	}()
}

// StopGenerate 中断指定会话的生成
func (a *App) StopGenerate(sessionID string) {
	a.sessionMu.Lock()
	if cancel, ok := a.sessionCancels[sessionID]; ok {
		cancel()
	}
	a.sessionMu.Unlock()
}

// ===== 模型配置 =====

func (a *App) GetModelConfig() (*config.AppConfig, error) {
	return config.Load()
}

func (a *App) SaveModelProviders(providers []config.ModelProvider) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Providers = providers
	return config.Save(cfg)
}

func (a *App) SetActiveModel(providerID string, model string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.ActiveModel = config.ActiveModel{
		ProviderID: providerID,
		Model:      model,
	}
	return config.Save(cfg)
}

// FetchProviderModels 从 API 拉取提供商的可用模型列表并自动保存
func (a *App) FetchProviderModels(providerID string) ([]string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	var provider *config.ModelProvider
	for i, p := range cfg.Providers {
		if p.ID == providerID {
			provider = &cfg.Providers[i]
			break
		}
	}
	if provider == nil {
		return nil, fmt.Errorf("未找到提供商: %s", providerID)
	}

	models, err := config.FetchModels(provider)
	if err != nil {
		return nil, err
	}

	// 只返回模型列表，不自动保存。用户在前端确认后手动点保存

	return models, nil
}

// ===== 技能管理 =====

func (a *App) GetSkills() []skill.Skill {
	return a.skillMgr.List()
}

// RefreshSkills 重新扫描磁盘加载技能（前端刷新按钮调用）
func (a *App) RefreshSkills() []skill.Skill {
	a.skillMgr.ReloadCustomSkills(a.ctx)
	return a.skillMgr.List()
}

func (a *App) InstallSkill(s skill.Skill) error {
	return a.skillMgr.Install(s)
}

func (a *App) InstallMCPSkill(s skill.Skill) error {
	return a.skillMgr.InstallMCP(a.ctx, s)
}

func (a *App) UninstallSkill(name string) error {
	return a.skillMgr.Uninstall(name)
}

func (a *App) SetSkillEnabled(name string, enabled bool) error {
	return a.skillMgr.SetEnabledWithCtx(a.ctx, name, enabled)
}

func (a *App) GetSkillDetail(name string) *skill.Skill {
	return a.skillMgr.Get(name)
}

// SearchClawHubSkills 搜索 clawhub.ai 技能（playwright）
func (a *App) SearchClawHubSkills(query string) ([]skill.ClawHubSkill, error) {
	return skill.SearchClawHub(query)
}

// InstallClawHubSkill 从 clawhub.ai 下载并安装技能（playwright）
func (a *App) InstallClawHubSkill(href string) error {
	if _, err := skill.InstallClawHubSkill(href); err != nil {
		return err
	}
	a.skillMgr.ReloadCustomSkills(a.ctx)
	return nil
}

// SearchSkillHubSkills 搜索 SkillHub(Tencent) 技能
func (a *App) SearchSkillHubSkills(query string) ([]skill.SkillHubSkill, error) {
	return skill.SearchSkillHub(query, 10)
}

// InstallSkillHubSkill 通过 skillhub CLI 安装技能，安装后重新加载
func (a *App) InstallSkillHubSkill(name string) error {
	if err := skill.InstallSkillHubSkill(name); err != nil {
		return err
	}
	a.skillMgr.ReloadCustomSkills(a.ctx)
	return nil
}

// ===== 定时任务 =====

func (a *App) GetScheduledTasks(sessionID string) []*ScheduledTask {
	return a.scheduler.ListTasks(sessionID)
}

func (a *App) GetAllScheduledTasks() []*ScheduledTask {
	return a.scheduler.ListAllTasks()
}

func (a *App) AddScheduledTask(task ScheduledTask) {
	a.scheduler.AddTask(&task)
}

func (a *App) UpdateScheduledTask(task ScheduledTask) {
	a.scheduler.UpdateTask(&task)
}

func (a *App) DeleteScheduledTask(taskID string) {
	a.scheduler.DeleteTask(taskID)
}

func (a *App) SetScheduledTaskEnabled(taskID string, enabled bool) {
	a.scheduler.SetTaskEnabled(taskID, enabled)
}

// ===== 执行追踪 =====

// GetExecutionTrace 获取某条消息的执行追踪
func (a *App) GetExecutionTrace(sessionID string, messageTs string) *ExecutionTrace {
	return getTrace(sessionID, messageTs)
}

// GetLatestTrace 获取某个 session 最新的执行追踪
func (a *App) GetLatestTrace(sessionID string) *ExecutionTrace {
	return getLatestTrace(sessionID)
}

// ===== 记忆系统状态 =====

func (a *App) GetMemoryStatus() map[string]any {
	return map[string]any{
		"embeddingAvailable": a.memMgr.IsEmbeddingAvailable(),
		"embeddingModelPath": a.memMgr.GetEmbeddingModelPath(),
	}
}

// ===== 渠道 =====

// GetChannels 获取所有渠道配置
func (a *App) GetChannels() []config.ChannelConfig {
	cfg, _ := config.Load()
	if cfg == nil {
		return nil
	}
	return cfg.Channels
}

// SaveChannel 保存渠道配置（新增或更新）
func (a *App) SaveChannel(ch config.ChannelConfig) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	found := false
	for i, c := range cfg.Channels {
		if c.ID == ch.ID {
			cfg.Channels[i] = ch
			found = true
			break
		}
	}
	if !found {
		cfg.Channels = append(cfg.Channels, ch)
	}
	if err := config.Save(cfg); err != nil {
		return err
	}

	// 保存后自动连接
	if ch.Enabled {
		go func() {
			if err := a.channelMgr.Connect(ch); err != nil {
				fmt.Printf("渠道自动连接失败(%s): %v\n", ch.Name, err)
			}
		}()
	}
	return nil
}

// DeleteChannel 删除渠道
func (a *App) DeleteChannel(id string) error {
	a.channelMgr.Disconnect(id)

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// 找到渠道绑定的助手，删除渠道后一并删除
	var botID string
	for i, c := range cfg.Channels {
		if c.ID == id {
			botID = c.BotID
			cfg.Channels = append(cfg.Channels[:i], cfg.Channels[i+1:]...)
			break
		}
	}
	if err := config.Save(cfg); err != nil {
		return err
	}

	// 自动删除渠道关联的助手
	if botID != "" {
		a.sessionMgr.Delete(botID)
	}
	return nil
}

// ConnectChannel 连接渠道
func (a *App) ConnectChannel(id string) error {
	cfg, _ := config.Load()
	if cfg == nil {
		return fmt.Errorf("配置加载失败")
	}
	for _, ch := range cfg.Channels {
		if ch.ID == id {
			return a.channelMgr.Connect(ch)
		}
	}
	return fmt.Errorf("渠道不存在: %s", id)
}

// DisconnectChannel 断开渠道
func (a *App) DisconnectChannel(id string) {
	a.channelMgr.Disconnect(id)
}

// GetChannelStatus 获取渠道连接状态
func (a *App) GetChannelStatus(id string) bool {
	return a.channelMgr.IsConnected(id)
}

// handleChannelMessage 处理渠道收到的消息（调用 Agent 生成回复）
func (a *App) handleChannelMessage(channelID, userID, text string) string {
	// 查找渠道绑定的助手
	cfg, _ := config.Load()
	if cfg == nil {
		return ""
	}
	var botID string
	for _, ch := range cfg.Channels {
		if ch.ID == channelID {
			botID = ch.BotID
			break
		}
	}
	if botID == "" {
		return "未绑定助手"
	}

	// 设置当前活跃会话（供 manage_schedule 等工具使用）
	a.activeSessionID = botID

	session := a.sessionMgr.Get(botID)
	if session == nil {
		return "助手不存在"
	}

	// 构建消息上下文
	contextMsgs, err := a.memMgr.BuildContext(botID, GetEffectivePrompt(session), text)
	if err != nil {
		return "构建上下文失败"
	}

	var messages []Message
	for _, m := range contextMsgs {
		messages = append(messages, Message{Role: m.Role, Content: m.Content})
	}
	if len(messages) > 0 && messages[0].Role == "system" {
		if s, ok := messages[0].Content.(string); ok {
			messages[0].Content = s + "\n\n" + a.skillMgr.GetSkillSummary()
		}
	}
	messages = append(messages, Message{Role: "user", Content: text})
	a.saveRequestLog(botID, messages)

	// 非流式收集完整回复
	var fullResponse string
	var llmOpts *LLMOptions
	if session.ProviderID != "" && session.Model != "" {
		llmOpts = &LLMOptions{ProviderID: session.ProviderID, Model: session.Model}
	}

	streamChat(context.Background(), messages, a.skillMgr, llmOpts,
		func(token string) { fullResponse += token },
		nil, nil,
	)

	// 存储消息
	a.sessionMgr.AppendMessage(botID, Message{Role: "user", Content: text})
	if fullResponse != "" {
		a.sessionMgr.AppendMessage(botID, Message{Role: "assistant", Content: fullResponse})
	}

	return fullResponse
}

// ===== 统计 =====

func (a *App) GetUsageStats() UsageStats {
	cfg, _ := config.Load()
	modelCount := 0
	if cfg != nil {
		for _, p := range cfg.Providers {
			modelCount += len(p.Models)
		}
	}
	skillCount := len(a.skillMgr.List())
	sessionCount := len(a.sessionMgr.List())
	return a.usageTracker.GetStats(modelCount, skillCount, sessionCount)
}

// ===== 审计 =====

func (a *App) GetAuditRecords(query audit.SkillQuery) audit.SkillPageResult {
	if a.auditDB == nil {
		return audit.SkillPageResult{Records: []audit.SkillRecord{}}
	}
	return a.auditDB.GetSkillRecords(query)
}

func (a *App) GetAuditStats(days int) audit.SkillStats {
	if a.auditDB == nil {
		return audit.SkillStats{ByTool: map[string]int{}, ByBot: map[string]int{}}
	}
	return a.auditDB.GetSkillStats(days)
}

func (a *App) GetStorageAuditRecords(query audit.StorageQuery) audit.StoragePageResult {
	if a.auditDB == nil {
		return audit.StoragePageResult{Records: []audit.StorageRecord{}}
	}
	return a.auditDB.GetStorageRecords(query)
}

func (a *App) GetStorageAuditStats(days int) audit.StorageStats {
	if a.auditDB == nil {
		return audit.StorageStats{ByType: map[string]int{}}
	}
	return a.auditDB.GetStorageStats(days)
}

// buildUserMessage 根据附件类型构建用户消息
func buildUserMessage(text string, attachments []Attachment) Message {
	if len(attachments) == 0 {
		return Message{Role: "user", Content: text}
	}

	hasImage := false
	var textContent strings.Builder
	textContent.WriteString(text)

	var imageParts []ContentPart

	for _, att := range attachments {
		switch att.Type {
		case "image":
			hasImage = true
			imageParts = append(imageParts, ContentPart{
				Type: "image_url",
				ImageURL: &ImageURL{
					URL: att.Content, // "data:image/png;base64,..."
				},
			})
		case "text":
			textContent.WriteString(fmt.Sprintf("\n\n--- 附件: %s ---\n%s", att.Name, att.Content))
		default:
			textContent.WriteString(fmt.Sprintf("\n\n[附件 %s 已上传，可通过 read_file 工具读取]", att.Name))
		}
	}

	// 有图片时使用多模态格式
	if hasImage {
		parts := []ContentPart{{Type: "text", Text: textContent.String()}}
		parts = append(parts, imageParts...)
		return Message{Role: "user", Content: parts}
	}

	return Message{Role: "user", Content: textContent.String()}
}
