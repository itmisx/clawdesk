package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"text/template"
	"time"
)

// parseJSON 解析 JSON 字符串
func parseJSON(jsonStr string, v any) error {
	return json.Unmarshal([]byte(jsonStr), v)
}

// openTerminal 在系统终端中执行命令（macOS 用 Terminal.app）
func openTerminal(cmdStr string, workDir string) ToolResult {
	if runtime.GOOS != "darwin" {
		// 非 macOS 回退到后台执行
		return executeCommandBackground(cmdStr, workDir)
	}

	// 构建终端命令：cd 到工作目录并执行
	var script string
	if workDir != "" {
		script = fmt.Sprintf(`cd %q && %s`, workDir, cmdStr)
	} else {
		script = cmdStr
	}

	// 用 osascript 在 Terminal.app 中打开新窗口执行
	osaCmd := exec.Command("osascript", "-e",
		fmt.Sprintf(`tell application "Terminal"
	activate
	do script %q
end tell`, script))
	if err := osaCmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("打开终端失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("已在终端中启动: %s", cmdStr), Success: true}
}

// executeCommandBackground 后台执行命令（不等待完成）
func executeCommandBackground(cmdStr string, workDir string) ToolResult {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", cmdStr)
	} else {
		cmd = exec.Command("bash", "-c", cmdStr)
	}
	if workDir != "" {
		cmd.Dir = workDir
	}
	if err := cmd.Start(); err != nil {
		return ToolResult{Output: fmt.Sprintf("后台启动失败: %v", err), Success: false}
	}
	go cmd.Wait() // 后台回收进程，不阻塞
	return ToolResult{Output: fmt.Sprintf("命令已在后台启动 (PID: %d)", cmd.Process.Pid), Success: true}
}

// executeCommand 执行 shell 命令模板
// command 是 Go template 格式，args 是 JSON 参数
// 模板中可以使用 {{.paramName}} 引用参数，{{.JSON}} 引用完整 JSON
// workDir 为空时使用当前目录
func executeCommand(commandTpl string, argsJSON string, workDirs ...string) ToolResult {
	var args map[string]any
	if err := parseJSON(argsJSON, &args); err != nil {
		return ToolResult{Output: fmt.Sprintf("参数解析失败: %v", err), Success: false}
	}

	// 添加完整 JSON 作为特殊变量
	args["JSON"] = argsJSON

	// 渲染命令模板
	tmpl, err := template.New("cmd").Parse(commandTpl)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("命令模板解析失败: %v", err), Success: false}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, args); err != nil {
		return ToolResult{Output: fmt.Sprintf("命令模板渲染失败: %v", err), Success: false}
	}

	cmdStr := buf.String()

	// 执行命令（2 分钟超时，防止阻塞工具调用循环）
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", cmdStr)
	}

	// 设置工作目录
	if len(workDirs) > 0 && workDirs[0] != "" {
		cmd.Dir = workDirs[0]
	}

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return ToolResult{Output: "命令执行超时（2分钟），请使用更轻量的方式完成任务", Success: false}
	}
	result := strings.TrimSpace(string(output))
	if err != nil {
		// exit code != 0，命令失败
		if result != "" {
			return ToolResult{Output: fmt.Sprintf("命令执行失败: %s\n输出: %s", err.Error(), result), Success: false}
		}
		return ToolResult{Output: fmt.Sprintf("命令执行失败: %s", err.Error()), Success: false}
	}

	if result == "" {
		return ToolResult{Output: "命令执行成功（无输出）", Success: true}
	}

	// 限制输出长度
	if len(result) > 8000 {
		result = result[:8000] + "\n... (输出被截断)"
	}

	return ToolResult{Output: result, Success: true}
}
