package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

// ModelProvider 模型厂商配置
type ModelProvider struct {
	ID      string   `json:"id" yaml:"id"`
	Name    string   `json:"name" yaml:"name"`       // 厂商名称，如 OpenAI, 阿里云, 腾讯云
	BaseURL string   `json:"baseUrl" yaml:"baseUrl"` // API地址
	APIKey  string   `json:"apiKey" yaml:"apiKey"`
	Models  []string `json:"models" yaml:"models"` // 可用模型列表
}

// ActiveModel 当前激活的模型
type ActiveModel struct {
	ProviderID string `json:"providerId" yaml:"providerId"`
	Model      string `json:"model" yaml:"model"`
}

// FeishuConfig 飞书渠道配置
type FeishuConfig struct {
	AppID      string `json:"appId" yaml:"appId"`
	AppSecret  string `json:"appSecret" yaml:"appSecret"`
	OpenID     string `json:"openId,omitempty" yaml:"openId,omitempty"`         // 推送目标用户 open_id（可选，手动填写或自动记录）
}

// WecomConfig 企业微信渠道配置
type WecomConfig struct {
	BotID      string `json:"botId" yaml:"botId"`
	Secret     string `json:"secret" yaml:"secret"`
	LastChatID string `json:"lastChatId,omitempty" yaml:"lastChatId,omitempty"` // 最近会话 ID（自动记录，用于主动推送）
}

// DingtalkConfig 钉钉渠道配置
type DingtalkConfig struct {
	ClientID     string `json:"clientId" yaml:"clientId"`         // AppKey（同时作为 robotCode 用于主动发消息）
	ClientSecret string `json:"clientSecret" yaml:"clientSecret"` // AppSecret
	LastUserID   string `json:"lastUserId,omitempty" yaml:"lastUserId,omitempty"` // 最近用户 staffId（自动记录，用于主动推送��
}

// ChannelConfig 渠道配置
type ChannelConfig struct {
	ID       string          `json:"id" yaml:"id"`
	Type     string          `json:"type" yaml:"type"`       // "feishu" | "wecom" | "dingtalk"
	Name     string          `json:"name" yaml:"name"`
	Enabled  bool            `json:"enabled" yaml:"enabled"`
	BotID    string          `json:"botId" yaml:"botId"`     // 绑定的助手会话 ID
	Feishu   *FeishuConfig   `json:"feishu,omitempty" yaml:"feishu,omitempty"`
	Wecom    *WecomConfig    `json:"wecom,omitempty" yaml:"wecom,omitempty"`
	Dingtalk *DingtalkConfig `json:"dingtalk,omitempty" yaml:"dingtalk,omitempty"`
}

// AppConfig 应用配置
type AppConfig struct {
	Providers   []ModelProvider `json:"providers" yaml:"providers"`
	ActiveModel ActiveModel     `json:"activeModel" yaml:"activeModel"`
	Channels    []ChannelConfig `json:"channels,omitempty" yaml:"channels,omitempty"`
}

var (
	configInstance *AppConfig
	configMu       sync.RWMutex
	configPath     string
)

func init() {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".clawdesk")
	os.MkdirAll(configDir, 0755)
	configPath = filepath.Join(configDir, "config.yaml")
}

// GetConfigDir 获取配置目录
func GetConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".clawdesk")
}

// Load 加载配置
func Load() (*AppConfig, error) {
	configMu.Lock()
	defer configMu.Unlock()

	if configInstance != nil {
		return configInstance, nil
	}

	configInstance = &AppConfig{
		Providers: defaultProviders(),
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 首次运行，保存默认配置
			return configInstance, save(configInstance)
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, configInstance); err != nil {
		return nil, err
	}

	// 合并预设提供商（确保新版本新增的提供商自动出现）
	mergeDefaultProviders(configInstance)

	return configInstance, nil
}

// Save 保存配置
func Save(cfg *AppConfig) error {
	configMu.Lock()
	defer configMu.Unlock()
	configInstance = cfg
	return save(cfg)
}

func save(cfg *AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// GetActiveProvider 获取当前激活的厂商配置
func GetActiveProvider(cfg *AppConfig) *ModelProvider {
	for i, p := range cfg.Providers {
		if p.ID == cfg.ActiveModel.ProviderID {
			return &cfg.Providers[i]
		}
	}
	if len(cfg.Providers) > 0 {
		return &cfg.Providers[0]
	}
	return nil
}

// GetActiveModelName 获取当前激活的模型名
func GetActiveModelName(cfg *AppConfig) string {
	if cfg.ActiveModel.Model != "" {
		return cfg.ActiveModel.Model
	}
	p := GetActiveProvider(cfg)
	if p != nil && len(p.Models) > 0 {
		return p.Models[0]
	}
	return ""
}

// Reload 强制重新加载
func Reload() (*AppConfig, error) {
	configMu.Lock()
	configInstance = nil
	configMu.Unlock()
	return Load()
}

// mergeDefaultProviders 合并预设提供商：新增缺失的、移除已废弃的
func mergeDefaultProviders(cfg *AppConfig) {
	defaults := defaultProviders()
	defaultIDs := make(map[string]bool)
	for _, d := range defaults {
		defaultIDs[d.ID] = true
	}

	// 记录已有的提供商 ID
	existingIDs := make(map[string]bool)
	for _, p := range cfg.Providers {
		existingIDs[p.ID] = true
	}

	// 添加缺失的预设提供商
	for _, d := range defaults {
		if !existingIDs[d.ID] {
			cfg.Providers = append(cfg.Providers, d)
		}
	}

	// 移除已废弃的预设提供商（用户未配置 APIKey 的才移除，已配置的保留）
	deprecatedIDs := map[string]bool{"deepseek": true, "aliyun": true}
	var kept []ModelProvider
	for _, p := range cfg.Providers {
		if deprecatedIDs[p.ID] && p.APIKey == "" {
			continue // 废弃且未配置，移除
		}
		kept = append(kept, p)
	}
	cfg.Providers = kept
}

func defaultProviders() []ModelProvider {
	return []ModelProvider{
		{
			ID:      "openai",
			Name:    "OpenAI",
			BaseURL: "https://api.openai.com/v1",
			APIKey:  "",
			Models:  []string{"gpt-4o", "gpt-4o-mini", "gpt-4", "gpt-3.5-turbo"},
		},
		{
			ID:      "anthropic",
			Name:    "Anthropic",
			BaseURL: "https://api.anthropic.com/v1",
			APIKey:  "",
			Models:  []string{"claude-sonnet-4-20250514", "claude-haiku-4-20250414", "claude-opus-4-20250514"},
		},
		{
			ID:      "moonshot",
			Name:    "Kimi (Moonshot)",
			BaseURL: "https://api.moonshot.cn/v1",
			APIKey:  "",
			Models:  []string{"kimi-k2.5", "kimi-k2-thinking", "kimi-k2-turbo-preview"},
		},
		{
			ID:      "qwen",
			Name:    "通义千问 (Qwen)",
			BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIKey:  "",
			Models:  []string{"qwen3.5-plus", "qwen3-coder-plus", "qwen3-coder-next"},
		},
		{
			ID:      "openrouter",
			Name:    "OpenRouter",
			BaseURL: "https://openrouter.ai/api/v1",
			APIKey:  "",
			Models:  []string{"openai/gpt-4o", "anthropic/claude-sonnet-4-20250514", "google/gemini-2.5-flash", "deepseek/deepseek-chat-v3-0324"},
		},
		{
			ID:      "ollama",
			Name:    "Ollama",
			BaseURL: "http://localhost:11434/v1",
			APIKey:  "ollama",
			Models:  []string{},
		},
		{
			ID:      "lm-studio",
			Name:    "LM Studio",
			BaseURL: "http://localhost:1234/v1",
			APIKey:  "lm-studio",
			Models:  []string{},
		},
	}
}

// FetchModels 从 API 拉取指定提供商的可用模型列表
func FetchModels(provider *ModelProvider) ([]string, error) {
	if provider.APIKey == "" {
		return nil, fmt.Errorf("API Key 未配置")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	baseURL := strings.TrimRight(provider.BaseURL, "/")

	// Anthropic 没有 /models 列表接口，返回预设列表
	if provider.ID == "anthropic" || strings.Contains(baseURL, "anthropic.com") {
		return []string{
			"claude-opus-4-20250514",
			"claude-sonnet-4-20250514",
			"claude-haiku-4-20250414",
			"claude-sonnet-4-5-20250514",
		}, nil
	}

	// OpenAI 兼容接口：GET /models
	req, err := http.NewRequest("GET", baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析失败: %w", err)
	}

	var models []string
	for _, m := range result.Data {
		id := m.ID
		// 只保留主要的聊天模型，过滤掉嵌入、TTS、whisper、DALL-E、旧版等
		if filterMainModel(id) {
			models = append(models, id)
		}
	}
	sort.Strings(models)
	return models, nil
}

// filterMainModel 过滤只保留主要聊天模型
func filterMainModel(id string) bool {
	// 对 OpenRouter 格式 "provider/model"，取 model 部分用于过滤
	lower := strings.ToLower(id)
	modelPart := lower
	if idx := strings.LastIndex(lower, "/"); idx >= 0 {
		modelPart = lower[idx+1:]
	}

	// 排除的关键词
	excludes := []string{
		"embed", "tts", "whisper", "dall-e", "davinci", "babbage",
		"curie", "ada", "moderation", "search", "similarity",
		"code-", "text-", "audio", "realtime", "transcribe",
		"instruct",
	}
	for _, ex := range excludes {
		if strings.Contains(modelPart, ex) {
			return false
		}
	}
	// 只保留包含这些关键词的模型
	includes := []string{"gpt", "o1", "o3", "o4", "claude", "chat", "deepseek", "qwen", "glm", "gemini", "mistral", "llama", "kimi", "moonshot"}
	for _, inc := range includes {
		if strings.Contains(modelPart, inc) {
			return true
		}
	}
	return false
}
