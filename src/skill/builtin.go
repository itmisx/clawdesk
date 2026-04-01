package skill

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// registerBuiltins 注册内置技能和工具执行器
func (m *Manager) registerBuiltins() {
	// 注册内置技能
	m.skills["system_tools"] = &Skill{
		Name:        "system_tools",
		DisplayName: "系统工具",
		Description: "内置的文件操作和命令执行工具",
		Version:     "1.0.0",
		Enabled:     true,
		Builtin:     true,
		Order:       0,
		Tools: []Tool{
			{
				Name:        "execute_command",
				Description: "在系统中执行 shell 命令。默认在当前会话的 workspace 目录下执行。设置 background=true 可后台运行（适用于 dev server 等常驻进程）。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"command":    {Type: "string", Description: "要执行的 shell 命令"},
						"workDir":    {Type: "string", Description: "工作目录（可选，默认使用会话 workspace）"},
						"background": {Type: "boolean", Description: "是否后台运行（可选，默认 false。npm start 等 dev server 应设为 true）"},
					},
					Required: []string{"command"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "read_file",
				Description: "读取指定路径的文本文件内容。支持绝对路径或相对于 workspace 的路径。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"path": {Type: "string", Description: "文件路径（绝对路径或相对于 workspace 的路径）"},
					},
					Required: []string{"path"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "write_file",
				Description: "将文本内容写入文件，如果文件或目录不存在则自动创建。优先使用 write_file 直接创建文件，而不是通过 execute_command 调用脚手架工具。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"path":    {Type: "string", Description: "文件路径（绝对路径或相对于 workspace 的路径）"},
						"content": {Type: "string", Description: "要写入的文本内容"},
					},
					Required: []string{"path", "content"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "list_directory",
				Description: "列出指定目录下的文件和文件夹",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"path": {Type: "string", Description: "目录路径（绝对路径或相对于 workspace 的路径），为空则列出 workspace"},
					},
					Required: []string{"path"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "fetch_url",
				Description: "抓取指定 URL 的网页内容，返回纯文本。可用于获取网页信息、API 数据、天气、新闻等",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"url":     {Type: "string", Description: "要抓取的完整 URL，如 https://example.com"},
						"method":  {Type: "string", Description: "HTTP 方法，默认 GET。可选 GET 或 POST"},
						"headers": {Type: "string", Description: "可选的请求头，JSON 格式，如 {\"Accept\": \"application/json\"}"},
						"body":    {Type: "string", Description: "POST 请求时的请求体"},
					},
					Required: []string{"url"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "plan_and_execute",
				Description: "当任务需要多个独立步骤并行执行时调用此工具（如同时查天气和查GitHub）。简单任务不要调用。参数为执行计划的 JSON。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"summary": {Type: "string", Description: "计划摘要，一句话描述要做什么"},
						"steps":   {Type: "string", Description: `步骤 JSON 数组，格式: [{"id":"1","name":"步骤名","description":"具体指令","dependsOn":[]}]。无依赖步骤会并行执行。`},
					},
					Required: []string{"summary", "steps"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "open_terminal",
				Description: "在系统终端中执行命令（适用于需要持续运行的服务，如 dev server）。会打开一个新的终端窗口执行命令，用户可以直接在终端中 Ctrl+C 停止。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"command": {Type: "string", Description: "要在终端中执行的命令，如 npm run dev"},
						"workDir": {Type: "string", Description: "工作目录（相对路径会自动解析到工作区）"},
					},
					Required: []string{"command"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "manage_schedule",
				Description: "管理当前助手的定时任务。支持查看、创建、停止、启用、删除定时任务。当用户要求定时执行、停止定时、查看定时任务等操作时调用。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"action":       {Type: "string", Description: "操作类型：list（查看所有）、create（创建）、stop（停止）、enable（启用）、delete（删除）"},
						"taskId":       {Type: "string", Description: "stop/enable/delete 时必填，任务 ID"},
						"name":         {Type: "string", Description: "create 时必填，任务名称"},
						"prompt":       {Type: "string", Description: "create 时必填，定时执行的提示词"},
						"scheduleType": {Type: "string", Description: "create 时，调度方式：interval 或 daily，默认 interval"},
						"interval":     {Type: "number", Description: "interval 模式的间隔分钟数，默认 30"},
						"dailyAt":      {Type: "string", Description: "daily 模式的执行时间 HH:MM"},
						"repeatType":   {Type: "string", Description: "重复方式：forever、count，默认 forever"},
						"repeatCount":  {Type: "number", Description: "count 模式的执行次数"},
					},
					Required: []string{"action"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "manage_skill",
				Description: "管理技能。支持搜索、安装、卸载、查看已安装技能。安装前必须先 search，不要假设技能已安装。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"action": {Type: "string", Description: "操作类型：list（查看已安装）、search（搜索技能）、install（安装技能）、uninstall（卸载技能）"},
						"query":  {Type: "string", Description: "search 时必填，搜索关键词"},
						"href":   {Type: "string", Description: "install 时必填，技能的 href 路径（从搜索结果获取）"},
						"name":   {Type: "string", Description: "uninstall 时必填，技能名称"},
					},
					Required: []string{"action"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "create_bot",
				Description: "创建一个新的 AI 助手/机器人。当用户要求新建、创建助手时调用此工具",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"name":         {Type: "string", Description: "助手名称，如 Translation Expert"},
						"avatar":       {Type: "string", Description: "助手头像 emoji，如 🤖💻📝🔍🎨"},
						"description":  {Type: "string", Description: "助手的简短描述"},
						"systemPrompt": {Type: "string", Description: "助手的系统提示词，定义其角色和行为"},
					},
					Required: []string{"name"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
		},
	}

	// 注册浏览器自动化技能（延迟加载，默认不发送工具定义）
	m.skills["browser_tools"] = &Skill{
		Name:        "browser_tools",
		DisplayName: "浏览器自动化",
		Description: "浏览器自动化工具集，支持导航、点击、填写、截图等操作",
		Version:     "1.0.0",
		Enabled:     true,
		Builtin:     true,
		Deferred:    true,
		Order:       1,
		Tools: []Tool{
			{Name: "use_browser", Description: "激活浏览器工具（第一步）。调用后浏览器操作工具变为可用，但浏览器不会打开，你必须紧接着调用 browser_navigate 打开网页。", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_navigate", Description: "在浏览器中打开指定 URL", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"url": {Type: "string", Description: "要打开的 URL"}}, Required: []string{"url"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_click", Description: "点击页面上的元素", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"selector": {Type: "string", Description: "CSS 选择器或文本选择器"}}, Required: []string{"selector"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_fill", Description: "在输入框中填写内容", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"selector": {Type: "string", Description: "输入框的 CSS 选择器"}, "value": {Type: "string", Description: "要填写的内容"}}, Required: []string{"selector", "value"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_select", Description: "选择下拉框选项", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"selector": {Type: "string", Description: "下拉框的 CSS 选择器"}, "value": {Type: "string", Description: "要选择的值"}}, Required: []string{"selector", "value"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_screenshot", Description: "对当前页面截图", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"fullPage": {Type: "string", Description: "是否全页截图，true/false，默认 false"}, "path": {Type: "string", Description: "保存路径，为空则自动生成"}}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_get_text", Description: "获取页面或指定元素的文本内容", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"selector": {Type: "string", Description: "CSS 选择器，为空则获取整个页面文本"}}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_get_html", Description: "获取页面或指定元素的 HTML", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"selector": {Type: "string", Description: "CSS 选择器，为空则获取整个页面 HTML"}}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_evaluate", Description: "在页面中执行 JavaScript 代码", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"expression": {Type: "string", Description: "要执行的 JS 表达式"}}, Required: []string{"expression"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_hover", Description: "悬停在指定元素上", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"selector": {Type: "string", Description: "CSS 选择器"}}, Required: []string{"selector"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_press_key", Description: "按下键盘按键", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"key": {Type: "string", Description: "按键名，如 Enter、Tab、ArrowDown"}}, Required: []string{"key"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_wait", Description: "等待指定元素出现", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"selector": {Type: "string", Description: "CSS 选择器"}, "timeout": {Type: "number", Description: "超时毫秒数，默认 10000"}}, Required: []string{"selector"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_go_back", Description: "浏览器后退", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_go_forward", Description: "浏览器前进", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_close", Description: "关闭当前浏览器页面", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_pdf", Description: "将当前页面保存为 PDF", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"path": {Type: "string", Description: "PDF 保存路径"}}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_upload", Description: "上传文件到指定的文件输入框", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"selector": {Type: "string", Description: "文件输入框的 CSS 选择器"}, "path": {Type: "string", Description: "要上传的文件路径"}}, Required: []string{"selector", "path"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_drag", Description: "拖拽元素到目标位置", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"source": {Type: "string", Description: "拖拽源的 CSS 选择器"}, "target": {Type: "string", Description: "拖拽目标的 CSS 选择器"}}, Required: []string{"source", "target"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_iframe_click", Description: "在 iframe 内点击元素", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"iframe": {Type: "string", Description: "iframe 的 CSS 选择器"}, "selector": {Type: "string", Description: "iframe 内元素的 CSS 选择器"}}, Required: []string{"iframe", "selector"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_iframe_fill", Description: "在 iframe 内填写输入框", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"iframe": {Type: "string", Description: "iframe 的 CSS 选择器"}, "selector": {Type: "string", Description: "iframe 内输入框的 CSS 选择器"}, "value": {Type: "string", Description: "要填写的内容"}}, Required: []string{"iframe", "selector", "value"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_click_new_tab", Description: "点击元素并切换到打开的新标签页", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"selector": {Type: "string", Description: "要点击的元素 CSS 选择器"}}, Required: []string{"selector"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_switch_tab", Description: "切换到指定索引的标签页", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"index": {Type: "number", Description: "标签页索引，从 0 开始"}}, Required: []string{"index"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_list_tabs", Description: "列出所有打开的标签页", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_set_user_agent", Description: "设置自定义 User-Agent（创建新标签页）", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"userAgent": {Type: "string", Description: "自定义 User-Agent 字符串"}}, Required: []string{"userAgent"}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_console_logs", Description: "获取浏览器控制台日志", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"search": {Type: "string", Description: "搜索关键词，为空则返回全部日志"}}, Required: []string{}}, Execute: ToolExecute{Type: "builtin"}},
			{Name: "browser_emulate_device", Description: "模拟移动设备（iPhone、iPad、Pixel 等）", Parameters: ToolParam{Type: "object", Properties: map[string]PropDef{"device": {Type: "string", Description: "设备名：iphone-14, iphone-14-pro, ipad-pro, pixel-7, galaxy-s23, desktop-1080p, desktop-1440p, laptop"}}, Required: []string{"device"}}, Execute: ToolExecute{Type: "builtin"}},
		},
	}

	// 注册内置工具执行器
	m.RegisterBuiltinExecutor("execute_command", func(args map[string]any) ToolResult {
		cmd, _ := args["command"].(string)
		if cmd == "" {
			return ToolResult{Output: "错误: command 参数为空", Success: false}
		}
		workDir, _ := args["workDir"].(string)
		if workDir == "" {
			workDir = m.getWorkspace()
		} else {
			workDir = m.resolveWorkspacePath(workDir)
		}
		bg, _ := args["background"].(bool)
		if bg {
			result := executeCommandBackground(cmd, workDir)
			result.Output = m.sanitizeOutput(result.Output)
			return result
		}
		result := executeCommand(cmd, "{}", workDir)
		result.Output = m.sanitizeOutput(result.Output)
		return result
	})

	m.RegisterBuiltinExecutor("open_terminal", func(args map[string]any) ToolResult {
		cmd, _ := args["command"].(string)
		if cmd == "" {
			return ToolResult{Output: "错误: command 参数为空", Success: false}
		}
		workDir, _ := args["workDir"].(string)
		if workDir == "" {
			workDir = m.getWorkspace()
		} else {
			workDir = m.resolveWorkspacePath(workDir)
		}
		return openTerminal(cmd, workDir)
	})

	m.RegisterBuiltinExecutor("read_file", func(args map[string]any) ToolResult {
		path, _ := args["path"].(string)
		if path == "" {
			return ToolResult{Output: "错误: path 参数为空", Success: false}
		}
		path = expandPath(m.resolveWorkspacePath(path))
		absPath, err := filepath.Abs(path)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("路径错误: %v", err), Success: false}
		}

		info, err := os.Stat(absPath)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("文件不存在: %v", err), Success: false}
		}
		if info.Size() > 10*1024*1024 {
			return ToolResult{Output: "文件过大（>10MB），不支持读取", Success: false}
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("读取失败: %v", err), Success: false}
		}
		content := string(data)
		if len(content) > 0 && !isTextContent(content) {
			return ToolResult{Output: "非文本文件，不支持读取", Success: false}
		}
		if len(content) > 8000 {
			content = content[:8000] + "\n... (内容被截断)"
		}
		return ToolResult{Output: content, Success: true}
	})

	m.RegisterBuiltinExecutor("write_file", func(args map[string]any) ToolResult {
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		if path == "" {
			return ToolResult{Output: "错误: path 参数为空", Success: false}
		}
		path = expandPath(m.resolveWorkspacePath(path))
		absPath, err := filepath.Abs(path)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("路径错误: %v", err), Success: false}
		}

		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return ToolResult{Output: fmt.Sprintf("创建目录失败: %v", err), Success: false}
		}

		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			return ToolResult{Output: fmt.Sprintf("写入失败: %v", err), Success: false}
		}
		displayPath := absPath
		if ws := m.getWorkspace(); ws != "" {
			if rel, err := filepath.Rel(ws, absPath); err == nil {
				displayPath = rel
			}
		}
		return ToolResult{Output: fmt.Sprintf("文件已成功写入: %s", displayPath), Success: true}
	})

	m.RegisterBuiltinExecutor("list_directory", func(args map[string]any) ToolResult {
		path, _ := args["path"].(string)
		if path == "" {
			path = m.getWorkspace()
			if path == "" {
				return ToolResult{Output: "错误: 无可用的工作区目录", Success: false}
			}
		} else {
			path = m.resolveWorkspacePath(path)
		}
		path = expandPath(path)
		absPath, err := filepath.Abs(path)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("路径错误: %v", err), Success: false}
		}

		entries, err := os.ReadDir(absPath)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("读取目录失败: %v", err), Success: false}
		}

		var sb strings.Builder
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if entry.IsDir() {
				fmt.Fprintf(&sb, "[目录] %s\n", entry.Name())
			} else {
				fmt.Fprintf(&sb, "[文件] %s (%d bytes)\n", entry.Name(), info.Size())
			}
		}
		if sb.Len() == 0 {
			return ToolResult{Output: "目录为空", Success: true}
		}
		return ToolResult{Output: sb.String(), Success: true}
	})

	m.RegisterBuiltinExecutor("fetch_url", func(args map[string]any) ToolResult {
		url, _ := args["url"].(string)
		if url == "" {
			return ToolResult{Output: "错误: url 参数为空", Success: false}
		}
		method, _ := args["method"].(string)
		if method == "" {
			method = "GET"
		}
		headersJSON, _ := args["headers"].(string)
		body, _ := args["body"].(string)

		return ToolResult{Output: fetchURL(url, method, headersJSON, body), Success: true}
	})

	// ===== 浏览器激活工具 executor =====
	m.RegisterBuiltinExecutor("use_browser", func(args map[string]any) ToolResult {
		return ToolResult{
			Output:  "浏览器工具已激活，但浏览器尚未打开。你必须接下来调用 browser_navigate 并传入 URL 才能打开网页。请立即调用 browser_navigate。",
			Success: true,
		}
	})

	// ===== 浏览器自动化 executor =====
	m.RegisterBuiltinExecutor("browser_navigate", func(args map[string]any) ToolResult {
		url, _ := args["url"].(string)
		if url == "" { return ToolResult{Output: "错误: url 参数为空", Success: false} }
		return BrowserNavigate(url)
	})
	m.RegisterBuiltinExecutor("browser_click", func(args map[string]any) ToolResult {
		selector, _ := args["selector"].(string)
		if selector == "" { return ToolResult{Output: "错误: selector 参数为空", Success: false} }
		return BrowserClick(selector)
	})
	m.RegisterBuiltinExecutor("browser_fill", func(args map[string]any) ToolResult {
		selector, _ := args["selector"].(string)
		value, _ := args["value"].(string)
		if selector == "" { return ToolResult{Output: "错误: selector 参数为空", Success: false} }
		return BrowserFill(selector, value)
	})
	m.RegisterBuiltinExecutor("browser_select", func(args map[string]any) ToolResult {
		selector, _ := args["selector"].(string)
		value, _ := args["value"].(string)
		if selector == "" { return ToolResult{Output: "错误: selector 参数为空", Success: false} }
		return BrowserSelect(selector, value)
	})
	m.RegisterBuiltinExecutor("browser_screenshot", func(args map[string]any) ToolResult {
		fullPage := false
		if v, _ := args["fullPage"].(string); v == "true" { fullPage = true }
		path, _ := args["path"].(string)
		return BrowserScreenshot(fullPage, path)
	})
	m.RegisterBuiltinExecutor("browser_get_text", func(args map[string]any) ToolResult {
		selector, _ := args["selector"].(string)
		return BrowserGetText(selector)
	})
	m.RegisterBuiltinExecutor("browser_get_html", func(args map[string]any) ToolResult {
		selector, _ := args["selector"].(string)
		return BrowserGetHTML(selector)
	})
	m.RegisterBuiltinExecutor("browser_evaluate", func(args map[string]any) ToolResult {
		expr, _ := args["expression"].(string)
		if expr == "" { return ToolResult{Output: "错误: expression 参数为空", Success: false} }
		return BrowserEvaluate(expr)
	})
	m.RegisterBuiltinExecutor("browser_hover", func(args map[string]any) ToolResult {
		selector, _ := args["selector"].(string)
		if selector == "" { return ToolResult{Output: "错误: selector 参数为空", Success: false} }
		return BrowserHover(selector)
	})
	m.RegisterBuiltinExecutor("browser_press_key", func(args map[string]any) ToolResult {
		key, _ := args["key"].(string)
		if key == "" { return ToolResult{Output: "错误: key 参数为空", Success: false} }
		return BrowserPressKey(key)
	})
	m.RegisterBuiltinExecutor("browser_wait", func(args map[string]any) ToolResult {
		selector, _ := args["selector"].(string)
		if selector == "" { return ToolResult{Output: "错误: selector 参数为空", Success: false} }
		timeout := 10000.0
		if v, ok := args["timeout"].(float64); ok { timeout = v }
		return BrowserWait(selector, timeout)
	})
	m.RegisterBuiltinExecutor("browser_go_back", func(args map[string]any) ToolResult {
		return BrowserGoBack()
	})
	m.RegisterBuiltinExecutor("browser_go_forward", func(args map[string]any) ToolResult {
		return BrowserGoForward()
	})
	m.RegisterBuiltinExecutor("browser_close", func(args map[string]any) ToolResult {
		return BrowserClose()
	})
	m.RegisterBuiltinExecutor("browser_pdf", func(args map[string]any) ToolResult {
		path, _ := args["path"].(string)
		return BrowserPDF(path)
	})
	m.RegisterBuiltinExecutor("browser_upload", func(args map[string]any) ToolResult {
		selector, _ := args["selector"].(string)
		path, _ := args["path"].(string)
		if selector == "" || path == "" { return ToolResult{Output: "错误: selector 和 path 参数必填", Success: false} }
		return BrowserUpload(selector, path)
	})
	m.RegisterBuiltinExecutor("browser_drag", func(args map[string]any) ToolResult {
		source, _ := args["source"].(string)
		target, _ := args["target"].(string)
		if source == "" || target == "" { return ToolResult{Output: "错误: source 和 target 参数必填", Success: false} }
		return BrowserDrag(source, target)
	})
	m.RegisterBuiltinExecutor("browser_iframe_click", func(args map[string]any) ToolResult {
		iframe, _ := args["iframe"].(string)
		selector, _ := args["selector"].(string)
		if iframe == "" || selector == "" { return ToolResult{Output: "错误: iframe 和 selector 参数必填", Success: false} }
		return BrowserIframeClick(iframe, selector)
	})
	m.RegisterBuiltinExecutor("browser_iframe_fill", func(args map[string]any) ToolResult {
		iframe, _ := args["iframe"].(string)
		selector, _ := args["selector"].(string)
		value, _ := args["value"].(string)
		if iframe == "" || selector == "" { return ToolResult{Output: "错误: iframe 和 selector 参数必填", Success: false} }
		return BrowserIframeFill(iframe, selector, value)
	})
	m.RegisterBuiltinExecutor("browser_click_new_tab", func(args map[string]any) ToolResult {
		selector, _ := args["selector"].(string)
		if selector == "" { return ToolResult{Output: "错误: selector 参数为空", Success: false} }
		return BrowserClickNewTab(selector)
	})
	m.RegisterBuiltinExecutor("browser_switch_tab", func(args map[string]any) ToolResult {
		index := 0
		if v, ok := args["index"].(float64); ok { index = int(v) }
		return BrowserSwitchTab(index)
	})
	m.RegisterBuiltinExecutor("browser_list_tabs", func(args map[string]any) ToolResult {
		return BrowserListTabs()
	})
	m.RegisterBuiltinExecutor("browser_set_user_agent", func(args map[string]any) ToolResult {
		ua, _ := args["userAgent"].(string)
		if ua == "" { return ToolResult{Output: "错误: userAgent 参数为空", Success: false} }
		return BrowserSetUserAgent(ua)
	})
	m.RegisterBuiltinExecutor("browser_console_logs", func(args map[string]any) ToolResult {
		search, _ := args["search"].(string)
		return BrowserConsoleLogs(search)
	})
	m.RegisterBuiltinExecutor("browser_emulate_device", func(args map[string]any) ToolResult {
		device, _ := args["device"].(string)
		if device == "" { return ToolResult{Output: "错误: device 参数为空", Success: false} }
		return BrowserEmulateDevice(device)
	})
}

// fetchURL 抓取 URL 内容
func fetchURL(url, method, headersJSON, body string) string {
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}

	req, err := http.NewRequest(strings.ToUpper(method), url, reqBody)
	if err != nil {
		return fmt.Sprintf("创建请求失败: %v", err)
	}
	req.Header.Set("User-Agent", "ClawDesk/1.0")

	// 解析自定义请求头
	if headersJSON != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersJSON), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("读取响应失败: %v", err)
	}

	result := string(data)

	// 如果是 HTML，简单去标签提取文本
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		result = stripHTML(result)
	}

	// 截断过长内容
	if len(result) > 8000 {
		result = result[:8000] + "\n... (内容被截断)"
	}

	if resp.StatusCode >= 400 {
		return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, result)
	}

	return result
}

// stripHTML 简单去除 HTML 标签
func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	inScript := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '<':
			inTag = true
			// 检测 <script 和 <style
			rest := strings.ToLower(s[i:])
			if strings.HasPrefix(rest, "<script") || strings.HasPrefix(rest, "<style") {
				inScript = true
			}
			if strings.HasPrefix(rest, "</script") || strings.HasPrefix(rest, "</style") {
				inScript = false
			}
		case s[i] == '>':
			inTag = false
		case !inTag && !inScript:
			result.WriteByte(s[i])
		}
	}
	// 合并多余空白
	text := result.String()
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

// expandPath 展开 ~ 为用户主目录
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	return path
}

// isTextContent 判断是否为文本内容
func isTextContent(content string) bool {
	checkLen := 512
	if len(content) < checkLen {
		checkLen = len(content)
	}
	for i := 0; i < checkLen; i++ {
		if content[i] == 0 {
			return false
		}
	}
	return true
}
