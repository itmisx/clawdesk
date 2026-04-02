package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"clawdesk/src/config"
	"clawdesk/src/skill"
)

// Message 消息结构，支持 tool 调用和多模态内容
type Message struct {
	Role             string     `json:"role"`                              // "system" | "user" | "assistant" | "tool"
	Content          any        `json:"content"`                           // string 或 []ContentPart（多模态）
	ReasoningContent string     `json:"reasoning_content,omitempty"`       // thinking 模型的推理内容（Kimi 等）
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`              // assistant 发起的工具调用
	ToolCallID       string     `json:"tool_call_id,omitempty"`           // tool 角色回复时的调用 ID
	Name             string     `json:"name,omitempty"`                   // tool 角色回复时的函数名
	Timestamp        string     `json:"timestamp,omitempty"`              // 消息时间（展示用）
}

// ContentPart 多模态内容块（OpenAI Vision 格式）
type ContentPart struct {
	Type     string    `json:"type"`               // "text" 或 "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL 图片 URL
type ImageURL struct {
	URL string `json:"url"` // "data:image/png;base64,..." 格式
}

// GetContentText 获取消息的文本内容（兼容 string 和 []ContentPart）
func (m *Message) GetContentText() string {
	switch v := m.Content.(type) {
	case string:
		return v
	case []any:
		for _, part := range v {
			if mp, ok := part.(map[string]any); ok {
				if mp["type"] == "text" {
					if text, ok := mp["text"].(string); ok {
						return text
					}
				}
			}
		}
	}
	return ""
}

// Attachment 用户上传的附件
type Attachment struct {
	Name    string `json:"name"`
	Type    string `json:"type"`    // "text" | "image" | "other"
	Content string `json:"content"` // 文本内容或 base64 数据
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用详情
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// streamChatResponse 单次流式请求的解析结果
type streamChatResponse struct {
	Content          string
	ReasoningContent string // Kimi thinking 模型的推理内容
	ToolCalls        []ToolCall
	Usage     TokenUsage
}

// LLMOptions 可选的模型覆盖参数（助手绑定模型时使用）
type LLMOptions struct {
	ProviderID string // 为空时使用全局配置
	Model      string
}

// doStreamRequestWithRetry 带重试的流式请求（最多重试 2 次，间隔 3 秒和 6 秒）
func doStreamRequestWithRetry(ctx context.Context, messages []Message, tools []map[string]any, onToken func(string), opts *LLMOptions) (*streamChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := doStreamRequest(ctx, messages, tools, onToken, opts)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		// 配置类错误不重试（未配置 API Key、未选择模型等）
		errMsg := err.Error()
		if strings.Contains(errMsg, "未配置") || strings.Contains(errMsg, "未选择") || strings.Contains(errMsg, "请先在") {
			return nil, err
		}
		// context 取消不重试
		if ctx.Err() != nil {
			return nil, err
		}
		// 等待后重试
		wait := time.Duration(3*(attempt+1)) * time.Second
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

// doStreamRequest 发起一次流式 LLM 请求
func doStreamRequest(ctx context.Context, messages []Message, tools []map[string]any, onToken func(string), opts *LLMOptions) (*streamChatResponse, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	// 确定使用哪个 provider 和 model
	var provider *config.ModelProvider
	var modelName string

	if opts != nil && opts.ProviderID != "" && opts.Model != "" {
		// 助手绑定了特定模型
		for i, p := range cfg.Providers {
			if p.ID == opts.ProviderID {
				provider = &cfg.Providers[i]
				break
			}
		}
		modelName = opts.Model
	}

	if provider == nil {
		provider = config.GetActiveProvider(cfg)
	}
	if modelName == "" {
		modelName = config.GetActiveModelName(cfg)
	}

	if provider == nil {
		return nil, fmt.Errorf("未配置模型厂商")
	}
	if provider.APIKey == "" {
		return nil, fmt.Errorf("请先在模型设置中配置 %s 的 API Key", provider.Name)
	}
	if modelName == "" {
		return nil, fmt.Errorf("未选择模型")
	}

	reqBody := map[string]any{
		"model":    modelName,
		"messages": messages,
		"stream":   true,
		"stream_options": map[string]any{
			"include_usage": true,
		},
	}
	if len(tools) > 0 {
		reqBody["tools"] = tools
	}

	body, _ := json.Marshal(reqBody)

	baseURL := strings.TrimRight(provider.BaseURL, "/")
	endpoint := baseURL + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]any
		json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("API 返回错误(%d): %v", resp.StatusCode, errBody)
	}

	result := &streamChatResponse{}
	toolCallMap := make(map[int]*ToolCall)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// 捕获 usage（通常在最后一个 chunk 或流式结束时）
		if chunk.Usage != nil {
			result.Usage = TokenUsage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.ReasoningContent != "" {
			result.ReasoningContent += delta.ReasoningContent
		}
		if delta.Content != "" {
			result.Content += delta.Content
			if onToken != nil {
				onToken(delta.Content)
			}
		}

		for _, tc := range delta.ToolCalls {
			existing, ok := toolCallMap[tc.Index]
			if !ok {
				existing = &ToolCall{ID: tc.ID, Type: tc.Type}
				toolCallMap[tc.Index] = existing
			}
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Type != "" {
				existing.Type = tc.Type
			}
			if tc.Function.Name != "" {
				existing.Function.Name += tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				existing.Function.Arguments += tc.Function.Arguments
			}
		}
	}

	for i := 0; i < len(toolCallMap); i++ {
		if tc, ok := toolCallMap[i]; ok {
			result.ToolCalls = append(result.ToolCalls, *tc)
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	return result, nil
}

// streamChat 流式聊天，支持 skill 工具调用循环
// skillMgr: 技能管理器，提供工具定义和执行
// onToken: 推送文本 token
// onToolCall: 通知前端正在调用哪个工具
func streamChat(ctx context.Context, messages []Message, skillMgr *skill.Manager, opts *LLMOptions, onToken func(string), onToolCall func(name string, args string), onToolResult func(name string, args string, result string, success bool, durationMs int64)) (TokenUsage, error) {
	activated := make(map[string]bool)
	tools := skillMgr.GetToolDefinitionsFiltered(activated)
	var totalUsage TokenUsage

	for i := 0; i < 10; i++ {
		resp, err := doStreamRequestWithRetry(ctx, messages, tools, onToken, opts)
		if err != nil {
			return totalUsage, err
		}

		// 累积 token 用量
		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		if len(resp.ToolCalls) == 0 {
			return totalUsage, nil
		}

		// 将 assistant 消息（含 tool_calls + reasoning_content）加入消息列表
		assistantMsg := Message{
			Role:             "assistant",
			Content:          resp.Content,
			ReasoningContent: resp.ReasoningContent,
			ToolCalls:        resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		// 同一轮多个 tool_calls 并发执行
		type toolCallResult struct {
			tc       ToolCall
			output   string
			success  bool
			duration int64
		}

		results := make([]toolCallResult, len(resp.ToolCalls))
		var wg sync.WaitGroup

		for i, tc := range resp.ToolCalls {
			if onToolCall != nil {
				onToolCall(tc.Function.Name, tc.Function.Arguments)
			}
			wg.Add(1)
			go func(idx int, call ToolCall) {
				defer wg.Done()
				start := time.Now()
				result := skillMgr.ExecuteTool(call.Function.Name, call.Function.Arguments)
				dur := time.Since(start).Milliseconds()
				results[idx] = toolCallResult{tc: call, output: result.Output, success: result.Success, duration: dur}
			}(i, tc)
		}
		wg.Wait()

		// 所有工具执行完毕，按顺序回调结果
		for _, r := range results {
			if onToolResult != nil {
				onToolResult(r.tc.Function.Name, r.tc.Function.Arguments, r.output, r.success, r.duration)
			}
		}

		// 检查延迟工具激活/停用，刷新工具列表
		needRefresh := false
		for _, r := range results {
			if r.tc.Function.Name == "use_browser" && r.success {
				activated["browser_tools"] = true
				needRefresh = true
			} else if r.tc.Function.Name == "browser_close" && r.success {
				delete(activated, "browser_tools")
				needRefresh = true
			}
		}
		if needRefresh {
			tools = skillMgr.GetToolDefinitionsFiltered(activated)
		}

		// 按顺序追加 tool messages（保持与 tool_calls 对应）
		for _, r := range results {
			toolMsg := Message{
				Role:       "tool",
				Content:    r.output,
				ToolCallID: r.tc.ID,
				Name:       r.tc.Function.Name,
			}
			messages = append(messages, toolMsg)
		}
		// 继续请求 LLM，让它根据工具结果生成回复
	}

	return totalUsage, fmt.Errorf("工具调用次数超过限制")
}

// DoNonStreamRequest 非流式 LLM 请求（供压缩器等内部功能调用）
func DoNonStreamRequest(ctx context.Context, messages []Message) (string, error) {
	resp, err := doNonStreamRequestRaw(ctx, messages, nil)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// NonStreamResponse 非流式请求的完整响应（含 tool_calls）
type NonStreamResponse struct {
	Content   string
	ToolCalls []ToolCall
}

// DoNonStreamRequestWithTools 带工具定义的非流式 LLM 请求
func DoNonStreamRequestWithTools(ctx context.Context, messages []Message, tools []map[string]any) (*NonStreamResponse, error) {
	return doNonStreamRequestRaw(ctx, messages, tools)
}

// doNonStreamRequestRaw 非流式请求底层实现
func doNonStreamRequestRaw(ctx context.Context, messages []Message, tools []map[string]any) (*NonStreamResponse, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	provider := config.GetActiveProvider(cfg)
	if provider == nil {
		return nil, fmt.Errorf("未配置模型厂商")
	}
	if provider.APIKey == "" {
		return nil, fmt.Errorf("请先配置 API Key")
	}

	modelName := config.GetActiveModelName(cfg)
	if modelName == "" {
		return nil, fmt.Errorf("未选择模型")
	}

	reqBody := map[string]any{
		"model":    modelName,
		"messages": messages,
		"stream":   false,
	}
	if len(tools) > 0 {
		reqBody["tools"] = tools
	}

	body, _ := json.Marshal(reqBody)
	baseURL := strings.TrimRight(provider.BaseURL, "/")
	endpoint := baseURL + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Choices) > 0 {
		return &NonStreamResponse{
			Content:   result.Choices[0].Message.Content,
			ToolCalls: result.Choices[0].Message.ToolCalls,
		}, nil
	}
	return nil, fmt.Errorf("LLM 返回空结果")
}
