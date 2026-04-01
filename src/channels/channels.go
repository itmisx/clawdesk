package channels

import (
	"context"
	"fmt"
	"sync"
	"time"

	"clawdesk/src/config"
)

// MessageHandler 消息处理回调：接收渠道ID、用户ID、消息文本，返回回复内容
type MessageHandler func(channelID, userID, text string) string

// SendFunc 主动发送消息函数
type SendFunc func(text string) error

// Connection 活跃的渠道连接
type Connection struct {
	ChannelID string
	Type      string // "feishu" | "wecom" | "dingtalk"
	cancel    context.CancelFunc
	send      SendFunc // 主动发送消息
}

// Manager 渠道管理器
type Manager struct {
	mu        sync.RWMutex
	conns     map[string]*Connection // channelID → 活跃连接
	retrying  map[string]bool        // 正在重连的渠道（防并发重连）
	onMessage MessageHandler
	stopped   bool
}

// NewManager 创建渠道管理器
func NewManager(onMessage MessageHandler) *Manager {
	return &Manager{
		conns:    make(map[string]*Connection),
		retrying: make(map[string]bool),
		onMessage: onMessage,
	}
}

// ConnectAll 连接所有已启用的渠道（应用启动时调用）
func (m *Manager) ConnectAll() {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return
	}
	for _, ch := range cfg.Channels {
		if ch.Enabled {
			go m.connectWithRetry(ch)
		}
	}
}

// Connect 建立渠道连接
func (m *Manager) Connect(cfg config.ChannelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已连接，先断开
	if conn, ok := m.conns[cfg.ID]; ok {
		conn.cancel()
		delete(m.conns, cfg.ID)
	}

	return m.connectLocked(cfg)
}

// connectLocked 内部连接（调用方需持有锁）
func (m *Manager) connectLocked(cfg config.ChannelConfig) error {
	ctx, cancel := context.WithCancel(context.Background())

	var err error
	var sendFn SendFunc

	switch cfg.Type {
	case "feishu":
		if cfg.Feishu == nil {
			cancel()
			return fmt.Errorf("飞书配置为空")
		}
		sendFn, err = connectFeishu(ctx, cfg, m.onMessage)
	case "wecom":
		if cfg.Wecom == nil {
			cancel()
			return fmt.Errorf("企业微信配置为空")
		}
		sendFn, err = connectWecom(ctx, cfg, m.onMessage, func() {
			m.onDisconnect(cfg)
		})
	case "dingtalk":
		if cfg.Dingtalk == nil {
			cancel()
			return fmt.Errorf("钉钉配置为空")
		}
		sendFn, err = connectDingtalk(ctx, cfg, m.onMessage, func() {
			m.onDisconnect(cfg)
		})
	default:
		cancel()
		return fmt.Errorf("未知渠道类型: %s", cfg.Type)
	}

	if err != nil {
		cancel()
		return err
	}

	m.conns[cfg.ID] = &Connection{
		ChannelID: cfg.ID,
		Type:      cfg.Type,
		cancel:    cancel,
		send:      sendFn,
	}
	return nil
}

// connectWithRetry 带重试的连接（后台 goroutine，同一渠道只允许一个）
func (m *Manager) connectWithRetry(cfg config.ChannelConfig) {
	m.mu.Lock()
	if m.retrying[cfg.ID] {
		m.mu.Unlock()
		return // 已有重连 goroutine 在运行
	}
	m.retrying[cfg.ID] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.retrying, cfg.ID)
		m.mu.Unlock()
	}()

	for attempt := 0; ; attempt++ {
		m.mu.RLock()
		stopped := m.stopped
		m.mu.RUnlock()
		if stopped {
			return
		}

		m.mu.Lock()
		err := m.connectLocked(cfg)
		m.mu.Unlock()

		if err == nil {
			return
		}

		// 指数退避：5s, 10s, 20s, 40s, 最长 60s
		wait := time.Duration(5<<uint(min(attempt, 4))) * time.Second
		if wait > 60*time.Second {
			wait = 60 * time.Second
		}
		time.Sleep(wait)
	}
}

// onDisconnect 连接断开时的回调，触发自动重连
func (m *Manager) onDisconnect(cfg config.ChannelConfig) {
	m.mu.Lock()
	delete(m.conns, cfg.ID)
	stopped := m.stopped
	m.mu.Unlock()

	if stopped {
		return
	}

	time.Sleep(3 * time.Second)
	go m.connectWithRetry(cfg)
}

// SendMessage 通过渠道主动发送消息
func (m *Manager) SendMessage(channelID, text string) error {
	m.mu.RLock()
	conn, ok := m.conns[channelID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("渠道未连接: %s", channelID)
	}
	if conn.send == nil {
		return fmt.Errorf("渠道不支持主动发送: %s", channelID)
	}
	return conn.send(text)
}

// SendToBot 根据助手 ID 查找绑定的渠道并发送消息
func (m *Manager) SendToBot(botID, text string) {
	cfg, _ := config.Load()
	if cfg == nil {
		return
	}
	for _, ch := range cfg.Channels {
		if ch.BotID == botID && ch.Enabled {
			if err := m.SendMessage(ch.ID, text); err != nil {
			}
		}
	}
}

// Disconnect 断开渠道连接
func (m *Manager) Disconnect(channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.conns[channelID]; ok {
		conn.cancel()
		delete(m.conns, channelID)
	}
}

// IsConnected 查询连接状态
func (m *Manager) IsConnected(channelID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.conns[channelID]
	return ok
}

// Shutdown 关闭所有连接
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopped = true
	for id, conn := range m.conns {
		conn.cancel()
		delete(m.conns, id)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
