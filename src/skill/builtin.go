package skill

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
				Name:        "web_search",
				Description: "搜索互联网获取实时信息。返回搜索结果的标题、URL 和摘要。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"query": {Type: "string", Description: "搜索关键词"},
						"limit": {Type: "number", Description: "返回结果数量（可选，默认 5，最大 10）"},
					},
					Required: []string{"query"},
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
				Name:        "glob_file",
				Description: "按文件名模式搜索文件路径。支持 glob 通配符，如 **/*.go、src/**/*.vue、*.md。返回匹配的文件路径列表。适合在不知道文件确切位置时快速定位文件。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"pattern": {Type: "string", Description: "glob 模式，如 **/*.go、src/**/*.ts、*.md"},
						"path":    {Type: "string", Description: "搜索根目录（可选，默认使用会话 workspace）"},
					},
					Required: []string{"pattern"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "grep_file",
				Description: "按正则表达式搜索文件内容。返回匹配的 文件:行号:内容。适合查找函数定义、变量引用、错误信息等在代码中的位置。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"pattern":        {Type: "string", Description: "正则表达式，如 func main、TODO.*fix、import.*react"},
						"path":           {Type: "string", Description: "搜索目录或文件路径（可选，默认使用会话 workspace）"},
						"include":        {Type: "string", Description: "文件名过滤 glob（可选），如 *.go、*.{ts,tsx}"},
						"caseSensitive":  {Type: "boolean", Description: "是否区分大小写（可选，默认 true）"},
					},
					Required: []string{"pattern"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "edit_file",
				Description: "精确替换文件中的文本片段，无需重写整个文件。old_text 必须与文件中的内容完全匹配（包括缩进和空白）。比 write_file 更安全高效，适合修改代码中的特定函数、变量或配置。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"path":       {Type: "string", Description: "文件路径（绝对路径或相对于 workspace 的路径）"},
						"old_text":   {Type: "string", Description: "要被替换的原始文本（必须与文件内容完全匹配）"},
						"new_text":   {Type: "string", Description: "替换后的新文本"},
						"replaceAll": {Type: "boolean", Description: "是否替换所有匹配项（可选，默认 false，只替换第一个）"},
					},
					Required: []string{"path", "old_text", "new_text"},
				},
				Execute: ToolExecute{Type: "builtin"},
			},
			{
				Name:        "file_tree",
				Description: "递归显示目录树结构。一次调用即可了解项目整体文件布局，比多次调用 list_directory 更高效。自动跳过 .git、node_modules 等目录。",
				Parameters: ToolParam{
					Type: "object",
					Properties: map[string]PropDef{
						"path":    {Type: "string", Description: "目录路径（可选，默认使用会话 workspace）"},
						"depth":   {Type: "number", Description: "递归深度（可选，默认 3，最大 6）"},
						"pattern": {Type: "string", Description: "文件名过滤 glob（可选），如 *.go、*.vue，为空则显示全部"},
					},
					Required: []string{},
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

	// ===== web_search executor =====
	m.RegisterBuiltinExecutor("web_search", func(args map[string]any) ToolResult {
		query, _ := args["query"].(string)
		if query == "" {
			return ToolResult{Output: "错误: query 参数为空", Success: false}
		}
		limit := 5
		if v, ok := args["limit"].(float64); ok && v >= 1 {
			limit = int(v)
			if limit > 10 {
				limit = 10
			}
		}
		results := webSearch(query, limit)
		if results == "" {
			return ToolResult{Output: "搜索无结果", Success: true}
		}
		return ToolResult{Output: results, Success: true}
	})

	// ===== file_tree executor =====
	m.RegisterBuiltinExecutor("file_tree", func(args map[string]any) ToolResult {
		root, _ := args["path"].(string)
		if root == "" {
			root = m.getWorkspace()
			if root == "" {
				return ToolResult{Output: "错误: 无可用的工作区目录", Success: false}
			}
		} else {
			root = m.resolveWorkspacePath(root)
		}
		root = expandPath(root)
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("路径错误: %v", err), Success: false}
		}

		maxDepth := 3
		if v, ok := args["depth"].(float64); ok && v >= 1 {
			maxDepth = int(v)
			if maxDepth > 6 {
				maxDepth = 6
			}
		}
		pattern, _ := args["pattern"].(string)

		result := buildFileTree(absRoot, maxDepth, pattern)
		result = m.sanitizeOutput(result)
		return ToolResult{Output: result, Success: true}
	})

	// ===== glob_file executor =====
	m.RegisterBuiltinExecutor("glob_file", func(args map[string]any) ToolResult {
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			return ToolResult{Output: "错误: pattern 参数为空", Success: false}
		}
		searchRoot, _ := args["path"].(string)
		if searchRoot == "" {
			searchRoot = m.getWorkspace()
			if searchRoot == "" {
				return ToolResult{Output: "错误: 无可用的工作区目录", Success: false}
			}
		} else {
			searchRoot = m.resolveWorkspacePath(searchRoot)
		}
		searchRoot = expandPath(searchRoot)
		absRoot, err := filepath.Abs(searchRoot)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("路径错误: %v", err), Success: false}
		}

		matches := globFiles(absRoot, pattern, 200)
		if len(matches) == 0 {
			return ToolResult{Output: "(无匹配文件)", Success: true}
		}
		result := strings.Join(matches, "\n")
		result = m.sanitizeOutput(result)
		return ToolResult{Output: result, Success: true}
	})

	// ===== grep_file executor =====
	m.RegisterBuiltinExecutor("grep_file", func(args map[string]any) ToolResult {
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			return ToolResult{Output: "错误: pattern 参数为空", Success: false}
		}
		searchPath, _ := args["path"].(string)
		if searchPath == "" {
			searchPath = m.getWorkspace()
			if searchPath == "" {
				return ToolResult{Output: "错误: 无可用的工作区目录", Success: false}
			}
		} else {
			searchPath = m.resolveWorkspacePath(searchPath)
		}
		searchPath = expandPath(searchPath)
		absPath, err := filepath.Abs(searchPath)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("路径错误: %v", err), Success: false}
		}

		include, _ := args["include"].(string)
		caseSensitive := true
		if v, ok := args["caseSensitive"].(bool); ok {
			caseSensitive = v
		}

		matches := grepFiles(absPath, pattern, include, caseSensitive, 200)
		if len(matches) == 0 {
			return ToolResult{Output: "(无匹配内容)", Success: true}
		}
		result := strings.Join(matches, "\n")
		result = m.sanitizeOutput(result)
		return ToolResult{Output: result, Success: true}
	})

	// ===== edit_file executor =====
	m.RegisterBuiltinExecutor("edit_file", func(args map[string]any) ToolResult {
		path, _ := args["path"].(string)
		oldText, _ := args["old_text"].(string)
		newText, _ := args["new_text"].(string)
		if path == "" {
			return ToolResult{Output: "错误: path 参数为空", Success: false}
		}
		if oldText == "" {
			return ToolResult{Output: "错误: old_text 参数为空", Success: false}
		}
		path = expandPath(m.resolveWorkspacePath(path))
		absPath, err := filepath.Abs(path)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("路径错误: %v", err), Success: false}
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("读取文件失败: %v", err), Success: false}
		}
		original := string(data)

		if !strings.Contains(original, oldText) {
			return ToolResult{Output: "错误: old_text 在文件中未找到，请确保文本完全匹配（包括缩进和空白）", Success: false}
		}

		replaceAll, _ := args["replaceAll"].(bool)
		var updated string
		if replaceAll {
			updated = strings.ReplaceAll(original, oldText, newText)
		} else {
			updated = strings.Replace(original, oldText, newText, 1)
		}

		if err := os.WriteFile(absPath, []byte(updated), 0644); err != nil {
			return ToolResult{Output: fmt.Sprintf("写入文件失败: %v", err), Success: false}
		}

		count := strings.Count(original, oldText)
		if replaceAll {
			displayPath := absPath
			if ws := m.getWorkspace(); ws != "" {
				if rel, err := filepath.Rel(ws, absPath); err == nil {
					displayPath = rel
				}
			}
			return ToolResult{Output: fmt.Sprintf("已更新 %s（替换了 %d 处）", displayPath, count), Success: true}
		}
		displayPath := absPath
		if ws := m.getWorkspace(); ws != "" {
			if rel, err := filepath.Rel(ws, absPath); err == nil {
				displayPath = rel
			}
		}
		return ToolResult{Output: fmt.Sprintf("已更新 %s（替换了 1 处）", displayPath), Success: true}
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

// skipDirs 需要跳过的目录名集合
var skipDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true,
	"node_modules": true, "__pycache__": true, "vendor": true,
	".venv": true, "venv": true, ".idea": true, ".vscode": true,
	"dist": true, "build": true, ".next": true, ".nuxt": true,
}

// buildFileTree 构建递归目录树
func buildFileTree(root string, maxDepth int, pattern string) string {
	var sb strings.Builder
	sb.WriteString(filepath.Base(root) + "/\n")
	buildTreeRecursive(&sb, root, "", maxDepth, 0, pattern, 0)
	lines := strings.Count(sb.String(), "\n")
	if lines > 500 {
		// 截断过大的输出
		result := sb.String()
		truncated := strings.Join(strings.SplitN(result, "\n", 502)[:500], "\n")
		return truncated + "\n... (输出被截断，请缩小 depth 或使用 pattern 过滤)"
	}
	return sb.String()
}

func buildTreeRecursive(sb *strings.Builder, dir, prefix string, maxDepth, currentDepth int, pattern string, count int) int {
	if currentDepth >= maxDepth || count > 500 {
		return count
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return count
	}

	// 过滤掉需要跳过的目录和隐藏文件
	var visible []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() && skipDirs[name] {
			continue
		}
		if pattern != "" && !e.IsDir() {
			matched, _ := filepath.Match(pattern, name)
			if !matched {
				continue
			}
		}
		visible = append(visible, e)
	}

	// 如果有 pattern 过滤，需要保留包含匹配文件的目录
	if pattern != "" {
		var filtered []os.DirEntry
		for _, e := range visible {
			if !e.IsDir() {
				filtered = append(filtered, e)
			} else {
				// 检查子目录是否有匹配文件
				if dirHasMatch(filepath.Join(dir, e.Name()), pattern, maxDepth-currentDepth-1) {
					filtered = append(filtered, e)
				}
			}
		}
		visible = filtered
	}

	for i, e := range visible {
		if count > 500 {
			break
		}
		isLast := i == len(visible)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}

		if e.IsDir() {
			fmt.Fprintf(sb, "%s%s%s/\n", prefix, connector, e.Name())
			count++
			count = buildTreeRecursive(sb, filepath.Join(dir, e.Name()), prefix+childPrefix, maxDepth, currentDepth+1, pattern, count)
		} else {
			fmt.Fprintf(sb, "%s%s%s\n", prefix, connector, e.Name())
			count++
		}
	}
	return count
}

// dirHasMatch 检查目录中是否有匹配 pattern 的文件
func dirHasMatch(dir, pattern string, remainDepth int) bool {
	if remainDepth < 0 {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || skipDirs[name] {
			continue
		}
		if !e.IsDir() {
			if matched, _ := filepath.Match(pattern, name); matched {
				return true
			}
		} else if dirHasMatch(filepath.Join(dir, name), pattern, remainDepth-1) {
			return true
		}
	}
	return false
}

// webSearch 使用 Bing 搜索
func webSearch(query string, limit int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	searchURL := "https://www.bing.com/search?q=" + urlEncode(query) + "&count=" + fmt.Sprintf("%d", limit)
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return fmt.Sprintf("创建搜索请求失败: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("搜索请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("读取搜索结果失败: %v", err)
	}

	return parseBingResults(string(body), limit)
}

// urlEncode 简单 URL 编码
func urlEncode(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' || b == '~' {
			sb.WriteByte(b)
		} else if b == ' ' {
			sb.WriteByte('+')
		} else {
			fmt.Fprintf(&sb, "%%%02X", b)
		}
	}
	return sb.String()
}

// parseBingResults 从 Bing 搜索页面解析结果
// Bing 结果结构: <li class="b_algo"><h2><a href="URL">TITLE</a></h2><p class="b_lineclamp...">SNIPPET</p></li>
func parseBingResults(html string, limit int) string {
	type searchResult struct {
		title, url, snippet string
	}
	var results []searchResult

	remaining := html
	for len(results) < limit {
		// 找到下一个搜索结果块
		algoIdx := strings.Index(remaining, `class="b_algo"`)
		if algoIdx == -1 {
			break
		}
		remaining = remaining[algoIdx:]

		// 找 <h2> 中的链接
		h2Idx := strings.Index(remaining, "<h2")
		if h2Idx == -1 {
			break
		}

		// 提取 href
		hrefIdx := strings.Index(remaining[h2Idx:], `href="`)
		if hrefIdx == -1 {
			break
		}
		hrefStart := h2Idx + hrefIdx + 6
		hrefEnd := strings.Index(remaining[hrefStart:], `"`)
		if hrefEnd == -1 {
			break
		}
		rawURL := htmlUnescape(remaining[hrefStart : hrefStart+hrefEnd])

		// 提取标题: href 后面找 > 到 </a>
		afterHref := remaining[hrefStart+hrefEnd:]
		aOpen := strings.Index(afterHref, ">")
		if aOpen == -1 {
			break
		}
		aClose := strings.Index(afterHref[aOpen:], "</a>")
		if aClose == -1 {
			break
		}
		title := stripHTMLTags(afterHref[aOpen+1 : aOpen+aClose])

		// 移到标题之后
		remaining = remaining[hrefStart+hrefEnd+aOpen+aClose:]

		// 提取摘要: 找 <p 或 <span 中的文本（在下一个 b_algo 之前）
		snippet := ""
		nextAlgo := strings.Index(remaining, `class="b_algo"`)
		searchScope := remaining
		if nextAlgo != -1 {
			searchScope = remaining[:nextAlgo]
		}
		// Bing 摘要常见 class: b_lineclamp, b_paractl, b_dList
		for _, marker := range []string{"b_lineclamp", "b_paractl", "b_dList"} {
			snipIdx := strings.Index(searchScope, marker)
			if snipIdx == -1 {
				continue
			}
			snipRemain := searchScope[snipIdx:]
			tagEnd := strings.Index(snipRemain, ">")
			if tagEnd == -1 {
				continue
			}
			// 找到对应的闭合标签（取前 500 字符范围）
			endScope := snipRemain[tagEnd+1:]
			if len(endScope) > 500 {
				endScope = endScope[:500]
			}
			closeIdx := strings.Index(endScope, "</p>")
			if closeIdx == -1 {
				closeIdx = strings.Index(endScope, "</span>")
			}
			if closeIdx == -1 {
				closeIdx = strings.Index(endScope, "</div>")
			}
			if closeIdx != -1 {
				snippet = stripHTMLTags(endScope[:closeIdx])
				break
			}
		}

		title = strings.TrimSpace(title)
		rawURL = strings.TrimSpace(rawURL)
		snippet = strings.TrimSpace(snippet)

		if title != "" && rawURL != "" && !strings.Contains(rawURL, "bing.com/ck/") {
			results = append(results, searchResult{title: title, url: rawURL, snippet: snippet})
		}
	}

	if len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n", i+1, r.title, r.url)
		if r.snippet != "" {
			fmt.Fprintf(&sb, "   %s\n", r.snippet)
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// stripHTMLTags 去除 HTML 标签
func stripHTMLTags(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// htmlUnescape 简单 HTML 实体解码
func htmlUnescape(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&#x27;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}

// urlDecode 简单 URL 解码
func urlDecode(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi := unhex(s[i+1])
			lo := unhex(s[i+2])
			if hi >= 0 && lo >= 0 {
				sb.WriteByte(byte(hi<<4 | lo))
				i += 2
				continue
			}
		} else if s[i] == '+' {
			sb.WriteByte(' ')
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return -1
	}
}

// globFiles 按 glob 模式搜索文件。优先使用 rg，回退到 filepath.WalkDir。
func globFiles(root, pattern string, limit int) []string {
	// 尝试用 ripgrep（自动尊重 .gitignore，跳过 .venv 等）
	if rgPath, err := exec.LookPath("rg"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, rgPath, "--files", "--glob", pattern, ".")
		cmd.Dir = root
		out, err := cmd.Output()
		if err == nil || cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			var matches []string
			scanner := bufio.NewScanner(bytes.NewReader(out))
			for scanner.Scan() && len(matches) < limit {
				line := strings.TrimSpace(scanner.Text())
				if line != "" {
					matches = append(matches, line)
				}
			}
			sort.Strings(matches)
			return matches
		}
	}

	// 回退: filepath.WalkDir + filepath.Match
	var matches []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(matches) >= limit {
			return filepath.SkipAll
		}
		// 跳过隐藏目录和常见大目录
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		matched, _ := filepath.Match(pattern, filepath.Base(rel))
		if !matched {
			// 也尝试用完整相对路径匹配（支持 **/ 模式的简化形式）
			matched, _ = filepath.Match(pattern, rel)
		}
		if matched {
			matches = append(matches, rel)
		}
		return nil
	})
	sort.Strings(matches)
	return matches
}

// grepFiles 按正则搜索文件内容。优先使用 rg，回退到纯 Go 实现。
func grepFiles(root, pattern, include string, caseSensitive bool, limit int) []string {
	// 尝试用 ripgrep
	if rgPath, err := exec.LookPath("rg"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		args := []string{"--no-heading", "--line-number", "--color", "never"}
		if !caseSensitive {
			args = append(args, "-i")
		}
		if include != "" {
			args = append(args, "--glob", include)
		}
		args = append(args, "--", pattern, ".")

		cmd := exec.CommandContext(ctx, rgPath, args...)
		cmd.Dir = root
		out, err := cmd.Output()
		if err == nil || cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			var matches []string
			scanner := bufio.NewScanner(bytes.NewReader(out))
			// 增大 scanner 缓冲区以处理长行
			scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
			for scanner.Scan() && len(matches) < limit {
				line := scanner.Text()
				if line != "" {
					matches = append(matches, line)
				}
			}
			return matches
		}
	}

	// 回退: 纯 Go 正则搜索
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return []string{fmt.Sprintf("正则表达式错误: %v", err)}
	}

	var matches []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if len(matches) >= limit {
			return filepath.SkipAll
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		// 文件名过滤
		if include != "" {
			matched, _ := filepath.Match(include, d.Name())
			if !matched {
				return nil
			}
		}
		// 跳过大文件
		info, err := d.Info()
		if err != nil || info.Size() > 2*1024*1024 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		// 跳过二进制文件
		if bytes.ContainsRune(data[:min(512, len(data))], 0) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		for lineNo, line := range strings.Split(string(data), "\n") {
			if len(matches) >= limit {
				break
			}
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, lineNo+1, line))
			}
		}
		return nil
	})
	return matches
}

