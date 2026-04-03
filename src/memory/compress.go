package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CompressSummary 压缩摘要
type CompressSummary struct {
	Summary      string    `json:"summary"`
	CoveredUntil time.Time `json:"coveredUntil"` // 已压缩到哪条消息的时间戳
	CreatedAt    time.Time `json:"createdAt"`
	MsgCount     int       `json:"msgCount"` // 已压缩的消息总数
}

// LLMCallFunc LLM 调用函数类型（避免循环依赖）
type LLMCallFunc func(ctx context.Context, systemPrompt string, userPrompt string) (string, error)

// Compressor 上下文压缩器
type Compressor struct {
	store     *DailyStore
	threshold int // 触发压缩的未压缩消息阈值
	llmCall   LLMCallFunc
}

func NewCompressor(store *DailyStore, threshold int, llmCall LLMCallFunc) *Compressor {
	return &Compressor{
		store:     store,
		threshold: threshold,
		llmCall:   llmCall,
	}
}

// ShouldCompress 判断是否需要压缩
// 逻辑：未被压缩的消息数 > threshold + recentKeep
func (c *Compressor) ShouldCompress(sessionID string, recentKeep int) bool {
	total, err := c.store.CountMessages(sessionID)
	if err != nil {
		return false
	}

	// 已压缩的消息数
	compressed := 0
	if summary, _ := c.LoadSummary(sessionID); summary != nil {
		compressed = summary.MsgCount
	}

	uncompressed := total - compressed
	return uncompressed > c.threshold+recentKeep
}

// Compress 增量压缩：只处理上次压缩之后的新消息
func (c *Compressor) Compress(ctx context.Context, sessionID string, recentKeep int) (*CompressSummary, error) {
	if c.llmCall == nil {
		return nil, fmt.Errorf("LLM 调用函数未配置")
	}

	allMsgs, err := c.store.LoadAllMessages(sessionID)
	if err != nil {
		return nil, err
	}
	if len(allMsgs) <= recentKeep {
		return nil, nil
	}

	// 要压缩的范围 = 全部 - 最近 recentKeep 条
	toCompress := allMsgs[:len(allMsgs)-recentKeep]
	if len(toCompress) == 0 {
		return nil, nil
	}

	// 加载已有摘要
	existingSummary, _ := c.LoadSummary(sessionID)

	// 找出上次压缩后的新消息
	var newMsgs []StoredMessage
	if existingSummary != nil && !existingSummary.CoveredUntil.IsZero() {
		for _, msg := range toCompress {
			if msg.Timestamp.After(existingSummary.CoveredUntil) {
				newMsgs = append(newMsgs, msg)
			}
		}
	} else {
		newMsgs = toCompress
	}

	if len(newMsgs) == 0 {
		return existingSummary, nil // 没有新消息需要压缩
	}

	// 构建压缩提示词（限制发给 LLM 的长度）
	var prompt strings.Builder
	if existingSummary != nil && existingSummary.Summary != "" {
		prompt.WriteString("以下是之前的对话摘要：\n")
		prompt.WriteString(existingSummary.Summary)
		if len([]rune(existingSummary.Summary)) > 1500 {
			prompt.WriteString("\n\n注意：旧摘要较长，请精简旧摘要内容，控制总字数在500字以内。\n")
		}
		prompt.WriteString("\n\n以下是新增的对话内容，请在原有摘要基础上整合为一份完整摘要：\n\n")
	} else {
		prompt.WriteString("请简洁总结以下对话内容，保留关键事实、决策和重要上下文：\n\n")
	}

	// 只取新消息，且限制总量避免超限
	charBudget := 4000 // 约 2000 中文字
	charUsed := 0
	for _, msg := range newMsgs {
		if msg.Role == "tool" {
			continue
		}
		line := fmt.Sprintf("[%s] %s\n", msg.Role, truncate(msg.Content, 300))
		if charUsed+len([]rune(line)) > charBudget {
			prompt.WriteString("...(部分消息已省略)\n")
			break
		}
		prompt.WriteString(line)
		charUsed += len([]rune(line))
	}

	summary, err := c.llmCall(ctx,
		"你是一个对话总结助手。请用简洁准确的语言总结对话内容，保留关键信息、用户偏好和重要决策。输出纯文本，不超过500字。",
		prompt.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("压缩调用 LLM 失败: %w", err)
	}

	// 硬截断：防止多轮增量压缩后摘要无限膨胀
	const maxSummaryRunes = 2000
	if len([]rune(summary)) > maxSummaryRunes {
		summary = string([]rune(summary)[:maxSummaryRunes]) + "..."
	}

	result := &CompressSummary{
		Summary:      summary,
		CoveredUntil: toCompress[len(toCompress)-1].Timestamp,
		CreatedAt:    time.Now(),
		MsgCount:     len(toCompress),
	}

	if err := c.SaveSummary(sessionID, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Compressor) LoadSummary(sessionID string) (*CompressSummary, error) {
	path := filepath.Join(c.store.sessionDir(sessionID), "summary.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var summary CompressSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (c *Compressor) SaveSummary(sessionID string, summary *CompressSummary) error {
	c.store.ensureSessionDir(sessionID)
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(c.store.sessionDir(sessionID), "summary.json")
	return os.WriteFile(path, data, 0644)
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}
