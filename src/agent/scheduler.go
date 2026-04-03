package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"clawdesk/src/config"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ===== 数据模型 =====

// ScheduledTask 定时任务
type ScheduledTask struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId"`  // 关联的助手
	Name      string         `json:"name"`       // 任务名称
	Prompt    string         `json:"prompt"`      // 执行的提示词
	Enabled   bool           `json:"enabled"`
	Schedule  ScheduleConfig `json:"schedule"`    // 调度配置
	Notify    NotifyConfig   `json:"notify"`      // 通知配置
	CreatedAt time.Time      `json:"createdAt"`

	// 运行时状态（不持久化）
	LastRunAt  time.Time `json:"lastRunAt"`
	RunCount   int       `json:"runCount"`
	LastResult string    `json:"lastResult"`
	LastError  string    `json:"lastError"`
}

// ScheduleConfig 调度配置
type ScheduleConfig struct {
	Type     string `json:"type"`     // "interval" | "daily"
	Interval int    `json:"interval"` // type=interval 时，间隔分钟数
	DailyAt  string `json:"dailyAt"`  // type=daily 时，每天执行时间 "HH:MM"

	RepeatType  string `json:"repeatType"`  // "forever" | "days" | "count"
	RepeatDays  int    `json:"repeatDays"`  // repeatType=days 时，持续天数
	RepeatCount int    `json:"repeatCount"` // repeatType=count 时，执行次数
}

// NotifyConfig 通知配置
type NotifyConfig struct {
	Enabled bool   `json:"enabled"`
	Type    string `json:"type"`    // "wecom" | "feishu"
	Webhook string `json:"webhook"` // Webhook URL
}

// ===== 调度器 =====

// Scheduler 定时任务调度器
type Scheduler struct {
	mu      sync.RWMutex
	tasks   map[string]*ScheduledTask // taskID -> task
	timers  map[string]context.CancelFunc
	app     *App // 用于执行任务
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewScheduler 创建调度器
func NewScheduler(app *App) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		tasks:  make(map[string]*ScheduledTask),
		timers: make(map[string]context.CancelFunc),
		app:    app,
		ctx:    ctx,
		cancel: cancel,
	}
	s.loadTasks()
	s.cleanupExpired()
	go s.cleanupLoop()
	return s
}

// ===== 持久化 =====

// scheduleFile 每个会话独立的定时任务文件
func scheduleFile(sessionID string) string {
	return filepath.Join(config.GetConfigDir(), "sessions", sessionID, "schedule.json")
}

func (s *Scheduler) loadTasks() {
	// 扫描所有会话目录加载定时任务
	sessionsDir := filepath.Join(config.GetConfigDir(), "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(scheduleFile(entry.Name()))
		if err != nil {
			continue
		}
		var tasks []*ScheduledTask
		if err := json.Unmarshal(data, &tasks); err != nil {
			continue
		}
		for _, t := range tasks {
			s.tasks[t.ID] = t
			if t.Enabled {
				s.startTask(t)
			}
		}
	}
}

// saveSessionTasks 保存某个会话的定时任务
func (s *Scheduler) saveSessionTasks(sessionID string) {
	var tasks []*ScheduledTask
	for _, t := range s.tasks {
		if t.SessionID == sessionID {
			tasks = append(tasks, t)
		}
	}
	filePath := scheduleFile(sessionID)
	if len(tasks) == 0 {
		os.Remove(filePath)
		return
	}
	os.MkdirAll(filepath.Dir(filePath), 0755)
	data, _ := json.MarshalIndent(tasks, "", "  ")
	os.WriteFile(filePath, data, 0644)
}

// ===== CRUD =====

// ListTasks 获取某个助手的所有定时任务
func (s *Scheduler) ListTasks(sessionID string) []*ScheduledTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*ScheduledTask
	for _, t := range s.tasks {
		if t.SessionID == sessionID {
			result = append(result, t)
		}
	}
	return result
}

// ListAllTasks 获取所有定时任务
func (s *Scheduler) ListAllTasks() []*ScheduledTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, t)
	}
	return result
}

// AddTask 添加定时任务
func (s *Scheduler) AddTask(task *ScheduledTask) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task.ID = fmt.Sprintf("task_%d", time.Now().UnixMilli())
	task.CreatedAt = time.Now()
	s.tasks[task.ID] = task
	s.saveSessionTasks(task.SessionID)

	if task.Enabled {
		s.startTask(task)
	}
}

// UpdateTask 更新定时任务
func (s *Scheduler) UpdateTask(task *ScheduledTask) {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, ok := s.tasks[task.ID]
	if !ok {
		return
	}

	s.stopTaskLocked(old.ID)

	task.CreatedAt = old.CreatedAt
	task.RunCount = old.RunCount
	s.tasks[task.ID] = task
	s.saveSessionTasks(task.SessionID)

	if task.Enabled {
		s.startTask(task)
	}
}

// DeleteTask 删除定时任务
func (s *Scheduler) DeleteTask(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return
	}
	sessionID := task.SessionID
	s.stopTaskLocked(taskID)
	delete(s.tasks, taskID)
	s.saveSessionTasks(sessionID)
}

// SetTaskEnabled 启用/禁用
func (s *Scheduler) SetTaskEnabled(taskID string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[taskID]
	if !ok {
		return
	}

	t.Enabled = enabled
	if enabled {
		s.startTask(t)
	} else {
		s.stopTaskLocked(taskID)
	}
	s.saveSessionTasks(t.SessionID)
}

// Shutdown 关闭调度器
func (s *Scheduler) Shutdown() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.timers {
		s.stopTaskLocked(id)
	}
}

// ===== 调度执行 =====

func (s *Scheduler) startTask(task *ScheduledTask) {
	ctx, cancel := context.WithCancel(s.ctx)
	s.timers[task.ID] = cancel

	go s.runLoop(ctx, task)
}

func (s *Scheduler) stopTaskLocked(taskID string) {
	if cancel, ok := s.timers[taskID]; ok {
		cancel()
		delete(s.timers, taskID)
	}
}

func (s *Scheduler) runLoop(ctx context.Context, task *ScheduledTask) {
	// 计算首次执行的等待时间
	var waitDuration time.Duration

	switch task.Schedule.Type {
	case "interval":
		waitDuration = time.Duration(task.Schedule.Interval) * time.Minute
	case "daily":
		waitDuration = s.timeUntilDaily(task.Schedule.DailyAt)
	default:
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(waitDuration):
			// 检查重复限制
			if s.isExpired(task) {
				s.autoDeleteTask(task)
				return
			}

			// 执行任务
			s.executeTask(task)

			// 执行后再次检查（count 模式下刚好达到上限）
			if s.isExpired(task) {
				s.autoDeleteTask(task)
				return
			}

			// 计算下一次等待
			switch task.Schedule.Type {
			case "interval":
				waitDuration = time.Duration(task.Schedule.Interval) * time.Minute
			case "daily":
				waitDuration = s.timeUntilDaily(task.Schedule.DailyAt)
			}
		}
	}
}

// autoDeleteTask 任务结束后自动删除并通知前端
func (s *Scheduler) autoDeleteTask(task *ScheduledTask) {
	s.mu.Lock()
	sessionID := task.SessionID
	taskName := task.Name
	s.stopTaskLocked(task.ID)
	delete(s.tasks, task.ID)
	s.saveSessionTasks(sessionID)
	s.mu.Unlock()

	fmt.Printf("定时任务已结束并自动删除: %s (%s)\n", taskName, task.ID)
	if s.app.ctx != nil {
		runtime.EventsEmit(s.app.ctx, "schedule:deleted", sessionID, task.ID, taskName)
	}
}

// cleanupExpired 扫描并删除所有已过期的任务
func (s *Scheduler) cleanupExpired() {
	s.mu.RLock()
	var expired []*ScheduledTask
	for _, t := range s.tasks {
		if s.isExpired(t) {
			expired = append(expired, t)
		}
	}
	s.mu.RUnlock()

	for _, t := range expired {
		s.autoDeleteTask(t)
	}
}

// cleanupLoop 定时扫描过期任务
func (s *Scheduler) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpired()
		}
	}
}

// isExpired 判断任务是否已过期（基于创建时间和执行次数）
func (s *Scheduler) isExpired(task *ScheduledTask) bool {
	switch task.Schedule.RepeatType {
	case "days":
		return time.Since(task.CreatedAt).Hours()/24 >= float64(task.Schedule.RepeatDays)
	case "count":
		return task.RunCount >= task.Schedule.RepeatCount
	default: // "forever"
		return false
	}
}

// timeUntilDaily 计算距离下一个 HH:MM 的时间
func (s *Scheduler) timeUntilDaily(dailyAt string) time.Duration {
	now := time.Now()
	var hour, min int
	fmt.Sscanf(dailyAt, "%d:%d", &hour, &min)

	target := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())
	if target.Before(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}

// executeTask 执行定时任务
func (s *Scheduler) executeTask(task *ScheduledTask) {
	s.mu.Lock()
	task.RunCount++
	task.LastRunAt = time.Now()
	s.mu.Unlock()

	// 执行 LLM 请求（非流式，同步获取结果）
	result, err := s.runPrompt(task.SessionID, task.Prompt)

	s.mu.Lock()
	if err != nil {
		task.LastError = err.Error()
		task.LastResult = ""
	} else {
		task.LastResult = result
		task.LastError = ""
	}
	s.saveSessionTasks(task.SessionID)
	s.mu.Unlock()

	// 通知前端聊天窗口刷新（如果用户正在看这个会话）
	if s.app.ctx != nil {
		runtime.EventsEmit(s.app.ctx, "schedule:done", task.SessionID, task.Name)
	}

	// 发送 Webhook 通知
	if task.Notify.Enabled && task.Notify.Webhook != "" {
		content := result
		if err != nil {
			content = "执行失败: " + err.Error()
		}
		sendNotification(task.Notify, task.Name, content)
	}

	// 推送到绑定该助手的渠道
	if s.app.channelMgr != nil && result != "" {
		s.app.channelMgr.SendToBot(task.SessionID, fmt.Sprintf("[定时任务] %s\n\n%s", task.Name, result))
	}
}

// runPrompt 在指定助手上下文中执行提示词
func (s *Scheduler) runPrompt(sessionID, prompt string) (string, error) {
	session := s.app.sessionMgr.Get(sessionID)
	if session == nil {
		return "", fmt.Errorf("助手不存在: %s", sessionID)
	}

	// 构建消息上下文
	contextMsgs, err := s.app.memMgr.BuildContext(sessionID, GetEffectivePrompt(session), prompt)
	if err != nil {
		return "", fmt.Errorf("构建上下文失败: %w", err)
	}

	var messages []Message
	for _, m := range contextMsgs {
		messages = append(messages, Message{Role: m.Role, Content: m.Content})
	}

	// 注入技能信息
	if len(messages) > 0 && messages[0].Role == "system" {
		if sys, ok := messages[0].Content.(string); ok {
			messages[0].Content = sys + "\n\n" + s.app.skillMgr.GetSkillSummary()
		}
	}
	messages = append(messages, Message{Role: "user", Content: prompt})
	s.app.saveRequestLog(sessionID, messages)

	// 构建模型覆盖参数（使用助手绑定的模型）
	var llmOpts *LLMOptions
	if session.ProviderID != "" && session.Model != "" {
		llmOpts = &LLMOptions{ProviderID: session.ProviderID, Model: session.Model}
	}

	// 非流式执行（定时任务不需要工具调用，避免意外触发 create_bot 等副作用）
	fullResponse, err := DoNonStreamRequest(s.ctx, messages, llmOpts)
	if err != nil {
		return "", err
	}

	// 保存消息到历史
	if fullResponse != "" {
		s.app.sessionMgr.AppendMessage(sessionID, Message{Role: "user", Content: prompt})
		s.app.sessionMgr.AppendMessage(sessionID, Message{Role: "assistant", Content: fullResponse})
	}

	return fullResponse, nil
}
