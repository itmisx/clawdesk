package skill

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MCPConfig MCP 服务器配置
type MCPConfig struct {
	Transport string            `yaml:"transport" json:"transport"` // "stdio" | "sse"
	Command   string            `yaml:"command" json:"command"`     // stdio: 启动命令
	Args      []string          `yaml:"args" json:"args"`           // stdio: 命令参数
	URL       string            `yaml:"url" json:"url"`             // sse: 服务器 URL
	Env       map[string]string `yaml:"env" json:"env"`             // 环境变量
}

// MCPTool MCP 服务器返回的工具定义
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// jsonRPCRequest JSON-RPC 2.0 请求
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCResponse JSON-RPC 2.0 响应
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPClient MCP 客户端
type MCPClient struct {
	config MCPConfig
	nextID atomic.Int64

	// stdio transport
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner

	// sse transport
	sseEndpoint string // POST 消息的目标 URL
	sseCancel   context.CancelFunc

	mu       sync.Mutex
	pending  map[int64]chan *jsonRPCResponse
	closed   bool
	readDone chan struct{}
}

// NewMCPClient 创建 MCP 客户端
func NewMCPClient(cfg MCPConfig) *MCPClient {
	return &MCPClient{
		config:   cfg,
		pending:  make(map[int64]chan *jsonRPCResponse),
		readDone: make(chan struct{}),
	}
}

// Connect 连接到 MCP 服务器并完成握手
func (c *MCPClient) Connect(ctx context.Context) error {
	switch c.config.Transport {
	case "stdio":
		return c.connectStdio(ctx)
	case "sse":
		return c.connectSSE(ctx)
	default:
		return fmt.Errorf("不支持的 MCP 传输方式: %s", c.config.Transport)
	}
}

// connectStdio 通过 stdio 连接 MCP 服务器
func (c *MCPClient) connectStdio(ctx context.Context) error {
	c.cmd = exec.CommandContext(ctx, c.config.Command, c.config.Args...)

	// 设置环境变量（继承父进程环境 + 自定义变量）
	if len(c.config.Env) > 0 {
		c.cmd.Env = os.Environ()
		for k, v := range c.config.Env {
			c.cmd.Env = append(c.cmd.Env, k+"="+v)
		}
	}

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("获取 stdin 失败: %w", err)
	}

	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("获取 stdout 失败: %w", err)
	}
	c.stdout = bufio.NewScanner(stdoutPipe)
	// MCP 消息可能很大
	c.stdout.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("启动 MCP 服务器失败: %w", err)
	}

	// 启动读取循环
	go c.readLoop()

	return c.initialize()
}

// connectSSE 通过 SSE 连接 MCP 服务器
func (c *MCPClient) connectSSE(ctx context.Context) error {
	sseCtx, cancel := context.WithCancel(ctx)
	c.sseCancel = cancel

	// 连接 SSE 端点获取 endpoint 事件
	req, err := http.NewRequestWithContext(sseCtx, "GET", c.config.URL, nil)
	if err != nil {
		cancel()
		return fmt.Errorf("创建 SSE 请求失败: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return fmt.Errorf("连接 SSE 失败: %w", err)
	}

	// 启动 SSE 读取循环
	go c.sseReadLoop(sseCtx, resp.Body)

	// 等待获取 endpoint（最多 10 秒）
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			cancel()
			return fmt.Errorf("等待 SSE endpoint 超时")
		case <-time.After(100 * time.Millisecond):
			c.mu.Lock()
			ep := c.sseEndpoint
			c.mu.Unlock()
			if ep != "" {
				return c.initialize()
			}
		}
	}
}

// readLoop stdio 读取循环
func (c *MCPClient) readLoop() {
	defer close(c.readDone)
	for c.stdout.Scan() {
		line := c.stdout.Text()
		if line == "" {
			continue
		}
		c.handleMessage([]byte(line))
	}
}

// sseReadLoop SSE 读取循环
func (c *MCPClient) sseReadLoop(ctx context.Context, body io.ReadCloser) {
	defer body.Close()
	defer close(c.readDone)

	scanner := bufio.NewScanner(body)
	var eventType string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		if after, ok := strings.CutPrefix(line, "event: "); ok {
			eventType = after
			continue
		}

		if data, ok := strings.CutPrefix(line, "data: "); ok {

			switch eventType {
			case "endpoint":
				// 解析 endpoint URL
				endpoint := data
				if !strings.HasPrefix(endpoint, "http") {
					// 相对路径，基于 SSE URL 构建
					base := c.config.URL
					if idx := strings.LastIndex(base, "/"); idx > 7 {
						base = base[:idx]
					}
					endpoint = base + "/" + strings.TrimPrefix(endpoint, "/")
				}
				c.mu.Lock()
				c.sseEndpoint = endpoint
				c.mu.Unlock()
			case "message":
				c.handleMessage([]byte(data))
			}
			eventType = ""
		}
	}
}

// handleMessage 处理收到的 JSON-RPC 消息
func (c *MCPClient) handleMessage(data []byte) {
	var resp jsonRPCResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return
	}

	// 匹配请求 ID
	if resp.ID == nil {
		return // 通知消息，忽略
	}

	var id int64
	switch v := resp.ID.(type) {
	case float64:
		id = int64(v)
	case json.Number:
		n, _ := v.Int64()
		id = n
	}

	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()

	if ok {
		ch <- &resp
	}
}

// sendRequest 发送 JSON-RPC 请求并等待响应
func (c *MCPClient) sendRequest(method string, params any) (*jsonRPCResponse, error) {
	id := c.nextID.Add(1)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan *jsonRPCResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP 客户端已关闭")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	if err := c.writeMessage(data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP 错误 %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-time.After(30 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP 请求超时: %s", method)
	}
}

// sendNotification 发送 JSON-RPC 通知（无 ID，无响应）
func (c *MCPClient) sendNotification(method string, params any) error {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return c.writeMessage(data)
}

// writeMessage 写入消息
func (c *MCPClient) writeMessage(data []byte) error {
	switch c.config.Transport {
	case "stdio":
		_, err := c.stdin.Write(append(data, '\n'))
		return err
	case "sse":
		c.mu.Lock()
		endpoint := c.sseEndpoint
		c.mu.Unlock()
		if endpoint == "" {
			return fmt.Errorf("SSE endpoint 未就绪")
		}
		resp, err := http.Post(endpoint, "application/json", strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("SSE POST 失败: HTTP %d", resp.StatusCode)
		}
		return nil
	default:
		return fmt.Errorf("不支持的传输方式: %s", c.config.Transport)
	}
}

// initialize MCP 握手
func (c *MCPClient) initialize() error {
	_, err := c.sendRequest("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "clawdesk",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("MCP initialize 失败: %w", err)
	}

	// 发送 initialized 通知
	return c.sendNotification("notifications/initialized", nil)
}

// ListTools 获取 MCP 服务器的工具列表
func (c *MCPClient) ListTools() ([]MCPTool, error) {
	resp, err := c.sendRequest("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("解析工具列表失败: %w", err)
	}

	return result.Tools, nil
}

// CallTool 调用 MCP 工具
func (c *MCPClient) CallTool(name string, args map[string]any) (string, error) {
	resp, err := c.sendRequest("tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("解析工具结果失败: %w", err)
	}

	var sb strings.Builder
	for _, c := range result.Content {
		if c.Type == "text" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(c.Text)
		}
	}

	text := sb.String()
	if result.IsError {
		return "", fmt.Errorf("MCP 工具执行失败: %s", text)
	}

	// 截断过长内容
	if len(text) > 8000 {
		text = text[:8000] + "\n... (内容被截断)"
	}

	return text, nil
}

// Close 关闭 MCP 客户端
func (c *MCPClient) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	// 清理所有等待中的请求
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()

	switch c.config.Transport {
	case "stdio":
		if c.stdin != nil {
			c.stdin.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			c.cmd.Process.Kill()
			c.cmd.Wait()
		}
	case "sse":
		if c.sseCancel != nil {
			c.sseCancel()
		}
	}

	// 等待读取循环结束
	select {
	case <-c.readDone:
	case <-time.After(3 * time.Second):
	}
}
