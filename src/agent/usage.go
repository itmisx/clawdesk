package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"clawdesk/src/config"
)

// TokenUsage 单次请求的 token 用量
type TokenUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

// UsageRecord 一条用量记录
type UsageRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	ProviderID string    `json:"providerId"`
	Model      string    `json:"model"`
	Usage      TokenUsage `json:"usage"`
}

// UsageStats 统计汇总
type UsageStats struct {
	TotalPromptTokens     int64                     `json:"totalPromptTokens"`
	TotalCompletionTokens int64                     `json:"totalCompletionTokens"`
	TotalTokens           int64                     `json:"totalTokens"`
	TotalRequests         int64                     `json:"totalRequests"`
	ByProvider            map[string]*ProviderUsage `json:"byProvider"`
	ModelCount            int                       `json:"modelCount"`
	SkillCount            int                       `json:"skillCount"`
	SessionCount          int                       `json:"sessionCount"`
}

// ProviderUsage 按厂商的用量统计
type ProviderUsage struct {
	ProviderID       string                `json:"providerId"`
	ProviderName     string                `json:"providerName"`
	PromptTokens     int64                 `json:"promptTokens"`
	CompletionTokens int64                 `json:"completionTokens"`
	TotalTokens      int64                 `json:"totalTokens"`
	Requests         int64                 `json:"requests"`
	ByModel          map[string]*ModelUsage `json:"byModel"`
}

// ModelUsage 按模型的用量统计
type ModelUsage struct {
	Model            string `json:"model"`
	PromptTokens     int64  `json:"promptTokens"`
	CompletionTokens int64  `json:"completionTokens"`
	TotalTokens      int64  `json:"totalTokens"`
	Requests         int64  `json:"requests"`
}

// UsageTracker token 用量追踪器
type UsageTracker struct {
	mu      sync.Mutex
	records []UsageRecord
	path    string
}

func NewUsageTracker() *UsageTracker {
	path := filepath.Join(config.GetConfigDir(), "usage.json")
	t := &UsageTracker{path: path}
	t.load()
	return t
}

func (ut *UsageTracker) load() {
	data, err := os.ReadFile(ut.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &ut.records)
}

func (ut *UsageTracker) save() {
	data, _ := json.MarshalIndent(ut.records, "", "  ")
	os.WriteFile(ut.path, data, 0644)
}

// Record 记录一次 token 用量
func (ut *UsageTracker) Record(providerID, model string, usage TokenUsage) {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	ut.records = append(ut.records, UsageRecord{
		Timestamp:  time.Now(),
		ProviderID: providerID,
		Model:      model,
		Usage:      usage,
	})
	ut.save()
}

// GetStats 获取统计汇总
func (ut *UsageTracker) GetStats(modelCount, skillCount, sessionCount int) UsageStats {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	stats := UsageStats{
		ByProvider:   make(map[string]*ProviderUsage),
		ModelCount:   modelCount,
		SkillCount:   skillCount,
		SessionCount: sessionCount,
	}

	// 加载厂商名称
	cfg, _ := config.Load()
	providerNames := make(map[string]string)
	if cfg != nil {
		for _, p := range cfg.Providers {
			providerNames[p.ID] = p.Name
		}
	}

	for _, r := range ut.records {
		stats.TotalPromptTokens += int64(r.Usage.PromptTokens)
		stats.TotalCompletionTokens += int64(r.Usage.CompletionTokens)
		stats.TotalTokens += int64(r.Usage.TotalTokens)
		stats.TotalRequests++

		// 按厂商
		pu, ok := stats.ByProvider[r.ProviderID]
		if !ok {
			pu = &ProviderUsage{
				ProviderID:   r.ProviderID,
				ProviderName: providerNames[r.ProviderID],
				ByModel:      make(map[string]*ModelUsage),
			}
			if pu.ProviderName == "" {
				pu.ProviderName = r.ProviderID
			}
			stats.ByProvider[r.ProviderID] = pu
		}
		pu.PromptTokens += int64(r.Usage.PromptTokens)
		pu.CompletionTokens += int64(r.Usage.CompletionTokens)
		pu.TotalTokens += int64(r.Usage.TotalTokens)
		pu.Requests++

		// 按模型
		mu, ok := pu.ByModel[r.Model]
		if !ok {
			mu = &ModelUsage{Model: r.Model}
			pu.ByModel[r.Model] = mu
		}
		mu.PromptTokens += int64(r.Usage.PromptTokens)
		mu.CompletionTokens += int64(r.Usage.CompletionTokens)
		mu.TotalTokens += int64(r.Usage.TotalTokens)
		mu.Requests++
	}

	return stats
}
