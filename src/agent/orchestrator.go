package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"clawdesk/src/skill"
)

// ===== 数据结构 =====

// TaskPlan 任务执行计划（Orchestrator 分解用户问题后生成）
type TaskPlan struct {
	Query    string      `json:"query"`    // 用户原始问题
	Summary  string      `json:"summary"`  // 计划摘要
	Steps    []*TaskStep `json:"steps"`    // 执行步骤
	Mermaid  string      `json:"mermaid"`  // 流程图（mermaid 语法）
	StartAt  time.Time   `json:"startAt"`
	EndAt    time.Time   `json:"endAt"`
	Duration int64       `json:"duration"` // 毫秒
}

// TaskStep 单个执行步骤
type TaskStep struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`        // 步骤名称
	Description string          `json:"description"`  // 步骤描述
	AgentRole   string          `json:"agentRole"`    // 子 Agent 角色，如 "researcher"、"executor"
	Status      string          `json:"status"`       // pending / running / done / failed
	ToolCalls   []TaskToolCall  `json:"toolCalls"`    // 该步骤的工具调用记录
	Result      string          `json:"result"`       // 步骤结果
	DependsOn   []string        `json:"dependsOn"`    // 依赖的步骤 ID
	StartAt     time.Time       `json:"startAt"`
	EndAt       time.Time       `json:"endAt"`
	Duration    int64           `json:"duration"`
}

// TaskToolCall 工具调用记录
type TaskToolCall struct {
	ToolName  string `json:"toolName"`
	Args      string `json:"args"`
	Result    string `json:"result"`
	Success   bool   `json:"success"`
	Duration  int64  `json:"duration"` // 毫秒
}

// ExecutionTrace 完整的执行追踪（存储在内存中，按 session+messageTs 索引）
type ExecutionTrace struct {
	SessionID string    `json:"sessionId"`
	MessageTs string    `json:"messageTs"` // 关联的用户消息时间戳
	Plan      *TaskPlan `json:"plan"`
}

// ===== 追踪存储 =====

var (
	traceStore   = make(map[string]*ExecutionTrace) // key: sessionId:messageTs
	traceStoreMu sync.RWMutex
)

func storeTrace(sessionID, messageTs string, trace *ExecutionTrace) {
	traceStoreMu.Lock()
	defer traceStoreMu.Unlock()
	traceStore[sessionID+":"+messageTs] = trace
}

func getTrace(sessionID, messageTs string) *ExecutionTrace {
	traceStoreMu.RLock()
	defer traceStoreMu.RUnlock()
	return traceStore[sessionID+":"+messageTs]
}

// getLatestTrace 获取某个 session 最新的执行追踪
func getLatestTrace(sessionID string) *ExecutionTrace {
	traceStoreMu.RLock()
	defer traceStoreMu.RUnlock()
	prefix := sessionID + ":"
	var latest *ExecutionTrace
	for k, v := range traceStore {
		if strings.HasPrefix(k, prefix) && v.Plan != nil {
			if latest == nil || v.Plan.StartAt.After(latest.Plan.StartAt) {
				latest = v
			}
		}
	}
	return latest
}

// ===== Orchestrator =====

// Orchestrator 多 Agent 编排器
type Orchestrator struct {
	skillMgr *skill.Manager
	opts     *LLMOptions

	// 回调
	onToken      func(string)
	onToolCall   func(string, string)
	onToolResult func(string, string, string, bool, int64)
	onStepStart  func(stepID, stepName string)
	onStepDone   func(stepID string)
	onFlush      func() // 每个阶段输出完毕，通知外部保存为独立消息并重置
}

// NewOrchestrator 创建编排器
func NewOrchestrator(skillMgr *skill.Manager, opts *LLMOptions) *Orchestrator {
	return &Orchestrator{skillMgr: skillMgr, opts: opts}
}

// planPrompt 让 LLM 分解任务的系统提示词
const planPrompt = `你是一个任务规划器。分析用户问题，判断是否需要多步骤执行。

如果问题简单（如打招呼、简单问答、单个工具就能完成），返回：
{"simple": true}

如果问题需要多个独立步骤（如同时查天气和查GitHub），返回执行计划：
{
  "simple": false,
  "summary": "计划摘要",
  "steps": [
    {"id": "1", "name": "步骤名", "description": "具体要做什么，包含必要参数", "agentRole": "executor", "dependsOn": []},
    {"id": "2", "name": "步骤名", "description": "具体要做什么", "agentRole": "executor", "dependsOn": ["1"]}
  ]
}

重要规则：
1. description 必须是可直接执行的具体指令，例如"用 gh repo list 命令获取当前用户的仓库列表并按 star 排序"而不是"查询用户的仓库"
2. 不要要求用户提供额外信息，默认使用当前环境（如 gh 默认使用已认证用户）
3. 能并行的步骤不要设置依赖关系（dependsOn 为空数组）
4. 只有确实需要前一步结果的才设置依赖
5. agentRole: researcher（信息收集）、executor（执行操作）、analyzer（分析汇总）

只返回 JSON，不要其他内容。`

// planResult LLM 返回的计划
type planResult struct {
	Simple  bool        `json:"simple"`
	Summary string      `json:"summary"`
	Steps   []planStep  `json:"steps"`
}

type planStep struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	AgentRole   string   `json:"agentRole"`
	DependsOn   []string `json:"dependsOn"`
}

// ExecutePlan 执行预定义的步骤计划（由 plan_and_execute 工具触发）
func (o *Orchestrator) ExecutePlan(ctx context.Context, messages []Message, summary string, steps []planStep) (*TaskPlan, TokenUsage, error) {
	taskPlan := &TaskPlan{
		Query:   summary,
		Summary: summary,
		StartAt: time.Now(),
	}

	for _, s := range steps {
		taskPlan.Steps = append(taskPlan.Steps, &TaskStep{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			AgentRole:   s.AgentRole,
			Status:      "pending",
			DependsOn:   s.DependsOn,
		})
	}

	return o.executePlan(ctx, messages, taskPlan, summary)
}

// Execute 执行用户请求（自动判断简单/复杂路径）— 保留向后兼容
func (o *Orchestrator) Execute(ctx context.Context, messages []Message, userInput string) (*TaskPlan, TokenUsage, error) {
	plan, err := o.makePlan(ctx, userInput, messages)
	if err != nil || plan.Simple {
		return nil, TokenUsage{}, nil
	}

	taskPlan := &TaskPlan{
		Query:   userInput,
		Summary: plan.Summary,
		StartAt: time.Now(),
	}

	for _, s := range plan.Steps {
		taskPlan.Steps = append(taskPlan.Steps, &TaskStep{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			AgentRole:   s.AgentRole,
			Status:      "pending",
			DependsOn:   s.DependsOn,
		})
	}

	return o.executePlan(ctx, messages, taskPlan, userInput)
}

// executePlan 实际执行计划（共享逻辑）
func (o *Orchestrator) executePlan(ctx context.Context, messages []Message, taskPlan *TaskPlan, userInput string) (*TaskPlan, TokenUsage, error) {

	// 输出执行计划（作为独立消息）
	if o.onToken != nil {
		o.onToken(fmt.Sprintf("📋 **执行计划**: %s\n\n", taskPlan.Summary))
		for _, s := range taskPlan.Steps {
			o.onToken(fmt.Sprintf("  %s. %s — %s\n", s.ID, s.Name, s.Description))
		}
	}
	if o.onFlush != nil {
		o.onFlush() // 计划作为一条独立消息
	}

	// Step 3: 按依赖顺序执行步骤
	var totalUsage TokenUsage
	completed := make(map[string]bool)
	stepResults := make(map[string]string)

	for {
		// 找出所有可执行的步骤（依赖已完成）
		var runnable []*TaskStep
		for _, step := range taskPlan.Steps {
			if step.Status != "pending" {
				continue
			}
			ready := true
			for _, dep := range step.DependsOn {
				if !completed[dep] {
					ready = false
					break
				}
			}
			if ready {
				runnable = append(runnable, step)
			}
		}

		if len(runnable) == 0 {
			break
		}

		// 并行执行，每个步骤完成后立即按序推送给前端
		type stepOutput struct {
			content string
			usage   TokenUsage
		}

		var wg sync.WaitGroup
		results := make([]stepOutput, len(runnable))
		doneCh := make([]chan struct{}, len(runnable))
		for i := range doneCh {
			doneCh[i] = make(chan struct{})
		}

		// 并行启动所有子 Agent
		for idx, step := range runnable {
			wg.Add(1)
			go func(i int, s *TaskStep) {
				defer wg.Done()
				defer close(doneCh[i])

				s.Status = "running"
				s.StartAt = time.Now()

				var mu sync.Mutex
				var content string

				subPrompt := o.buildSubAgentPrompt(s, stepResults, userInput)
				// 系统提示词（含技能信息）
				sysContent := ""
				if len(messages) > 0 && messages[0].Role == "system" {
					if sys, ok := messages[0].Content.(string); ok {
						sysContent = sys
					}
				}
				subMessages := []Message{
					{Role: "system", Content: sysContent + "\n\n" + subPrompt},
				}
				// 注入最近历史（让子 Agent 了解对话上下文）
				subMessages = append(subMessages, extractRecentHistory(messages, 4)...)
				// 当前子任务指令
				subMessages = append(subMessages, Message{Role: "user", Content: s.Description})

				usage, err := streamChat(ctx, subMessages, o.skillMgr, o.opts,
					func(token string) {
						mu.Lock()
						content += token
						mu.Unlock()
					},
					func(name, args string) {
						if o.onToolCall != nil {
							o.onToolCall(name, args)
						}
						mu.Lock()
						s.ToolCalls = append(s.ToolCalls, TaskToolCall{ToolName: name, Args: args})
						mu.Unlock()
					},
					func(name, args, result string, success bool, dur int64) {
						if o.onToolResult != nil {
							o.onToolResult(name, args, result, success, dur)
						}
						mu.Lock()
						if len(s.ToolCalls) > 0 {
							last := &s.ToolCalls[len(s.ToolCalls)-1]
							last.Result = result
							last.Success = success
							last.Duration = dur
						}
						mu.Unlock()
					},
				)

				s.EndAt = time.Now()
				s.Duration = s.EndAt.Sub(s.StartAt).Milliseconds()

				if err != nil {
					s.Status = "failed"
					s.Result = err.Error()
				} else {
					s.Status = "done"
					s.Result = content
				}

				results[i] = stepOutput{content: content, usage: usage}
			}(idx, step)
		}

		// 按顺序等待并立即输出每个步骤（步骤 1 完成就输出 1，即使 2 还在跑）
		for i, s := range runnable {
			<-doneCh[i] // 等这个步骤完成

			totalUsage.PromptTokens += results[i].usage.PromptTokens
			totalUsage.CompletionTokens += results[i].usage.CompletionTokens
			totalUsage.TotalTokens += results[i].usage.TotalTokens

			completed[s.ID] = true
			stepResults[s.ID] = s.Result

			// 立即推送该步骤结果（每步作为独立消息）
			if o.onToken != nil {
				o.onToken(fmt.Sprintf("🔄 **步骤 %s: %s**\n\n", s.ID, s.Name))
				if results[i].content != "" {
					o.onToken(results[i].content)
				}
				status := "✅"
				if s.Status == "failed" {
					status = "❌"
				}
				o.onToken(fmt.Sprintf("\n\n%s **步骤 %s 完成** (%dms)", status, s.ID, s.Duration))
			}
			if o.onFlush != nil {
				o.onFlush() // 该步骤作为一条独立消息
			}
		}
	}

	// Step 4: 汇总所有步骤结果，生成最终回复
	if o.onToken != nil {
		o.onToken("---\n\n📝 **汇总**\n\n")
	}

	summaryPrompt := fmt.Sprintf("用户问题：%s\n\n以下是各步骤的执行结果，请综合所有信息给出完整的最终回答：\n\n", userInput)
	for _, s := range taskPlan.Steps {
		status := "成功"
		if s.Status == "failed" {
			status = "失败"
		}
		summaryPrompt += fmt.Sprintf("【%s】(%s)：%s\n", s.Name, status, s.Result)
	}

	// 汇总 Agent 携带系统提示词（含技能）+ 历史 + 步骤结果
	sysSummary := "你是一个汇总助手。根据各步骤执行结果，给出简洁完整的最终回答。不要重复罗列原始数据，直接给出结论。"
	if len(messages) > 0 && messages[0].Role == "system" {
		if sys, ok := messages[0].Content.(string); ok {
			sysSummary = sys + "\n\n" + sysSummary
		}
	}
	summaryMsgs := []Message{
		{Role: "system", Content: sysSummary},
	}
	summaryMsgs = append(summaryMsgs, extractRecentHistory(messages, 4)...)
	summaryMsgs = append(summaryMsgs, Message{Role: "user", Content: summaryPrompt})

	summaryUsage, summaryErr := streamChat(ctx, summaryMsgs, o.skillMgr, o.opts,
		o.onToken, o.onToolCall, o.onToolResult,
	)
	totalUsage.PromptTokens += summaryUsage.PromptTokens
	totalUsage.CompletionTokens += summaryUsage.CompletionTokens
	totalUsage.TotalTokens += summaryUsage.TotalTokens

	if summaryErr != nil && o.onToken != nil {
		o.onToken("\n\n汇总生成失败: " + summaryErr.Error())
	}

	if o.onToken != nil {
		o.onToken("\n\n")
	}

	taskPlan.EndAt = time.Now()
	taskPlan.Duration = taskPlan.EndAt.Sub(taskPlan.StartAt).Milliseconds()
	taskPlan.Mermaid = o.generateMermaid(taskPlan)

	return taskPlan, totalUsage, nil
}

// makePlan 让 LLM 分析问题并生成执行计划（携带技能信息和历史上下文）
func (o *Orchestrator) makePlan(ctx context.Context, userInput string, messages []Message) (*planResult, error) {
	// 构建规划器上下文：planPrompt + 可用技能列表 + 最近历史
	sysContent := planPrompt
	if o.skillMgr != nil {
		sysContent += "\n\n可用工具/技能:\n" + o.skillMgr.GetSkillSummary()
	}

	msgs := []Message{
		{Role: "system", Content: sysContent},
	}

	// 注入最近几条历史消息（让规划器了解对话上下文）
	history := extractRecentHistory(messages, 4)
	msgs = append(msgs, history...)

	msgs = append(msgs, Message{Role: "user", Content: userInput})

	resp, err := DoNonStreamRequest(ctx, msgs)
	if err != nil {
		return nil, err
	}

	var result planResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return &planResult{Simple: true}, nil
	}
	return &result, nil
}

// extractRecentHistory 从消息列表中提取最近 N 条 user/assistant 消息
func extractRecentHistory(messages []Message, n int) []Message {
	var history []Message
	for _, m := range messages {
		if m.Role == "user" || m.Role == "assistant" {
			if s, ok := m.Content.(string); ok && s != "" {
				history = append(history, Message{Role: m.Role, Content: s})
			}
		}
	}
	if len(history) > n {
		history = history[len(history)-n:]
	}
	return history
}

// buildSubAgentPrompt 为子 Agent 构建专属提示词
func (o *Orchestrator) buildSubAgentPrompt(step *TaskStep, prevResults map[string]string, userQuery string) string {
	prompt := fmt.Sprintf(`
=== 子任务执行指令 ===
用户原始问题: %s
你正在执行其中一个子任务。

当前任务: %s
说明: %s

执行要求:
1. 你必须立即调用工具来完成任务，不要询问用户补充信息
2. 如果缺少参数（如用户名），使用工具先获取或使用默认值（如 gh 命令默认使用当前认证用户）
3. 当已安装技能的同名工具可用时（如 github、weather），必须调用该工具，参考系统提示词中的 SKILL.md 说明构造 command 参数
4. 只完成当前子任务，简洁输出结果
`, userQuery, step.Name, step.Description)

	if len(step.DependsOn) > 0 {
		prompt += "\n前置步骤结果:\n"
		for _, depID := range step.DependsOn {
			if r, ok := prevResults[depID]; ok {
				prompt += fmt.Sprintf("步骤 %s 结果:\n%s\n", depID, r)
			}
		}
	}

	return prompt
}

// generateMermaid 生成执行流程的 mermaid 图
func (o *Orchestrator) generateMermaid(plan *TaskPlan) string {
	m := "graph TD\n"
	m += "    START([用户提问]) --> PLAN[任务规划]\n"

	// 收集哪些步骤是叶子节点（没有被其他步骤依赖）
	hasDep := make(map[string]bool) // 被依赖的步骤
	for _, s := range plan.Steps {
		for _, dep := range s.DependsOn {
			hasDep[dep] = true
		}
	}

	for _, s := range plan.Steps {
		icon := "✅"
		if s.Status == "failed" {
			icon = "❌"
		}
		nodeID := "S" + s.ID
		m += fmt.Sprintf("    %s[\"%s %s<br/>%dms\"]\n", nodeID, icon, s.Name, s.Duration)

		// 连接：有依赖则从依赖步骤连，无依赖则从 PLAN 连
		if len(s.DependsOn) == 0 {
			m += fmt.Sprintf("    PLAN --> %s\n", nodeID)
		} else {
			for _, dep := range s.DependsOn {
				m += fmt.Sprintf("    S%s --> %s\n", dep, nodeID)
			}
		}

		// 工具调用作为子节点
		for j, tc := range s.ToolCalls {
			tcID := fmt.Sprintf("%s_T%d", nodeID, j)
			tcIcon := "🔧"
			if !tc.Success {
				tcIcon = "⚠️"
			}
			m += fmt.Sprintf("    %s --> %s(%s %s)\n", nodeID, tcID, tcIcon, tc.ToolName)
		}
	}

	// 叶子步骤（不被任何步骤依赖的）连到 RESULT
	for _, s := range plan.Steps {
		if !hasDep[s.ID] {
			m += fmt.Sprintf("    S%s --> RESULT([汇总回复])\n", s.ID)
		}
	}

	return m
}
