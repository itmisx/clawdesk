package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

// ===== Playwright & 浏览器生命周期管理 =====

var (
	pwInstance *playwright.Playwright
	pwInstOnce sync.Once
	pwInstErr  error

	// 无头浏览器（ClawHub 内部搜索等）
	headlessBrowser   playwright.Browser
	headlessBrowserMu sync.Mutex

	// 有头浏览器（用户交互 browser_* 工具）
	headedBrowser   playwright.Browser
	headedContext   playwright.BrowserContext // PersistentContext 引用（保留 cookies）
	headedBrowserMu sync.Mutex
)

// ensurePlaywright 确保 playwright 已安装并运行
func ensurePlaywright() (*playwright.Playwright, error) {
	pwInstOnce.Do(func() {
		if err := playwright.Install(&playwright.RunOptions{Browsers: []string{"chromium"}}); err != nil {
			pwInstErr = fmt.Errorf("安装 Chromium 失败: %w", err)
			return
		}
		pw, err := playwright.Run()
		if err != nil {
			pwInstErr = fmt.Errorf("启动 Playwright 失败: %w", err)
			return
		}
		pwInstance = pw
	})
	return pwInstance, pwInstErr
}

// ensureBrowser 获取无头浏览器（内部用，如 ClawHub 搜索）
func ensureBrowser() (playwright.Browser, error) {
	headlessBrowserMu.Lock()
	defer headlessBrowserMu.Unlock()

	if headlessBrowser != nil {
		return headlessBrowser, nil
	}

	pw, err := ensurePlaywright()
	if err != nil {
		return nil, err
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("启动无头浏览器失败: %w", err)
	}
	headlessBrowser = browser
	return headlessBrowser, nil
}

// cdpDebugPort Chrome 远程调试端口
const cdpDebugPort = "9222"

// isChromeDebuggingAvailable 检测本地是否有 Chrome 开启了远程调试端口
func isChromeDebuggingAvailable() bool {
	resp, err := httpClient.Get("http://127.0.0.1:" + cdpDebugPort + "/json/version")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// ensureHeadedBrowser 获取有头浏览器（用户交互 browser_* 工具）
// 优先级：
//  1. CDP 连接已有调试端口的 Chrome（用户自己启动的带 --remote-debugging-port 的 Chrome）
//  2. 独立 profile 启动系统 Chrome（不关闭用户 Chrome，登录状态持久化在 ~/.clawdesk/cache/chrome-profile/）
//  3. 兜底：Playwright 自带浏览器
func ensureHeadedBrowser() (playwright.Browser, error) {
	headedBrowserMu.Lock()
	defer headedBrowserMu.Unlock()

	if headedBrowser != nil {
		return headedBrowser, nil
	}

	pw, err := ensurePlaywright()
	if err != nil {
		return nil, err
	}

	// 1. 尝试 CDP 连接
	if isChromeDebuggingAvailable() {
		browser, err := pw.Chromium.ConnectOverCDP("http://127.0.0.1:" + cdpDebugPort)
		if err == nil {
			headedBrowser = browser
			setupBrowserDisconnectHandler(headedBrowser)
			fmt.Println("已连接到运行中的 Chrome（CDP）")
			return headedBrowser, nil
		}
	}

	// 2. 使用独立 profile 启动 Chrome（不关闭用户 Chrome，登录状态首次需手动登录，后续自动保留）
	profileDir := browserProfileDir()
	ctx, err := pw.Chromium.LaunchPersistentContext(profileDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(false),
		Channel:  playwright.String("chrome"),
	})
	if err == nil {
		headedBrowser = ctx.Browser()
		headedContext = ctx
		setupBrowserDisconnectHandler(headedBrowser)
		fmt.Println("已启动 Chrome（独立 profile）")
		return headedBrowser, nil
	}
	fmt.Printf("独立 profile 启动失败: %v\n", err)

	// 3. 兜底
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false),
		Channel:  playwright.String("chrome"),
	})
	if err != nil {
		return nil, fmt.Errorf("启动浏览器失败: %w", err)
	}
	headedBrowser = browser
	setupBrowserDisconnectHandler(headedBrowser)
	return headedBrowser, nil
}

// browserProfileDir 返回浏览器自动化专用的 profile 目录（持久化，登录一次后续复用）
func browserProfileDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".clawdesk", "cache", "chrome-profile")
	os.MkdirAll(dir, 0755)
	return dir
}


// ShutdownBrowser 关闭所有 playwright 浏览器（保留 profile 目录以复用登录状态）
func ShutdownBrowser() {
	if headlessBrowser != nil {
		headlessBrowser.Close()
		headlessBrowser = nil
	}
	if headedBrowser != nil {
		headedBrowser.Close()
		headedBrowser = nil
		headedContext = nil
	}
	activePageMu.Lock()
	activePages = nil
	activePageMu.Unlock()
}

// ===== 浏览器页面管理（支持多标签页）=====

var (
	activePages   []playwright.Page // 页面栈，最后一个是当前活跃页
	activePageMu  sync.Mutex
	consoleLogs   []string // 控制台日志缓存
	consoleLogsMu sync.Mutex
)

// getOrCreatePage 获取或创建浏览器页面（自动清理已关闭的页面）
func getOrCreatePage() (playwright.Page, error) {
	activePageMu.Lock()
	defer activePageMu.Unlock()

	// 清理已关闭的页面
	var alive []playwright.Page
	for _, p := range activePages {
		if !p.IsClosed() {
			alive = append(alive, p)
		}
	}
	activePages = alive

	if len(activePages) > 0 {
		return activePages[len(activePages)-1], nil
	}

	// 检查浏览器是否还活着，不活着则重建
	browser, err := ensureHeadedBrowser()
	if err != nil {
		return nil, err
	}

	// 检查浏览器连接是否有效
	if !browser.IsConnected() {
		// 浏览器已断开，重置并重建
		headedBrowserMu.Lock()
		headedBrowser = nil
		headedBrowserMu.Unlock()
		browser, err = ensureHeadedBrowser()
		if err != nil {
			return nil, err
		}
	}

	// 复用浏览器中已有的空白页（如 Chrome 启动时的 new tab），非空白页不占用
	for _, ctx := range browser.Contexts() {
		for _, p := range ctx.Pages() {
			if p.IsClosed() {
				continue
			}
			url := p.URL()
			if url == "" || url == "about:blank" || url == "chrome://newtab/" || url == "chrome://new-tab-page/" {
				setupConsoleListener(p)
				activePages = append(activePages, p)
				return p, nil
			}
		}
	}

	// 创建新页面（优先从 PersistentContext 创建，保留 cookies）
	var page playwright.Page
	if headedContext != nil {
		page, err = headedContext.NewPage()
	} else {
		page, err = browser.NewPage()
	}
	if err != nil {
		return nil, fmt.Errorf("创建页面失败: %w", err)
	}
	setupConsoleListener(page)
	activePages = append(activePages, page)
	return page, nil
}

// setupConsoleListener 监听页面控制台日志
func setupConsoleListener(page playwright.Page) {
	page.On("console", func(msg playwright.ConsoleMessage) {
		consoleLogsMu.Lock()
		defer consoleLogsMu.Unlock()
		entry := fmt.Sprintf("[%s] %s", msg.Type(), msg.Text())
		consoleLogs = append(consoleLogs, entry)
		if len(consoleLogs) > 200 {
			consoleLogs = consoleLogs[len(consoleLogs)-200:]
		}
	})

	// 监听页面关闭事件，自动清理
	page.On("close", func() {
		activePageMu.Lock()
		defer activePageMu.Unlock()
		var alive []playwright.Page
		for _, p := range activePages {
			if !p.IsClosed() {
				alive = append(alive, p)
			}
		}
		activePages = alive
	})
}

// setupBrowserDisconnectHandler 监听浏览器断开，清理所有页面和浏览器引用
func setupBrowserDisconnectHandler(browser playwright.Browser) {
	browser.On("disconnected", func() {
		activePageMu.Lock()
		activePages = nil
		activePageMu.Unlock()

		headedBrowserMu.Lock()
		headedBrowser = nil
		headedContext = nil
		headedBrowserMu.Unlock()
	})
}

// closePage 关闭当前页面（如有多个则切回上一个）
func closePage() {
	activePageMu.Lock()
	defer activePageMu.Unlock()
	if len(activePages) > 0 {
		activePages[len(activePages)-1].Close()
		activePages = activePages[:len(activePages)-1]
	}
}

// ===== 浏览器工具函数 =====

// safeBrowserOp 安全执行浏览器操作（检测浏览器是否已断开）
func safeBrowserOp(fn func(playwright.Page) ToolResult) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: "浏览器未就绪: " + err.Error(), Success: false}
	}
	if page.IsClosed() {
		// 页面已关闭，清理并重试一次
		activePageMu.Lock()
		activePages = nil
		activePageMu.Unlock()
		page, err = getOrCreatePage()
		if err != nil {
			return ToolResult{Output: "浏览器重建失败: " + err.Error(), Success: false}
		}
	}

	// 用 channel + goroutine 包装，防止 Playwright 操作无限阻塞
	ch := make(chan ToolResult, 1)
	go func() {
		ch <- fn(page)
	}()

	select {
	case result := <-ch:
		return result
	case <-time.After(35 * time.Second):
		// 超时，浏览器可能被手动关闭
		activePageMu.Lock()
		activePages = nil
		activePageMu.Unlock()
		return ToolResult{Output: "操作超时，浏览器可能已关闭。再次操作将自动重新打开。", Success: false}
	}
}

// BrowserNavigate 导航到 URL
func BrowserNavigate(url string) ToolResult {
	return safeBrowserOp(func(page playwright.Page) ToolResult {
		if _, err := page.Goto(url, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			Timeout:   playwright.Float(30000),
		}); err != nil {
			return ToolResult{Output: fmt.Sprintf("导航失败: %v", err), Success: false}
		}
		title, _ := page.Title()
		return ToolResult{Output: fmt.Sprintf("已打开: %s\n标题: %s", url, title), Success: true}
	})
}

// BrowserClick 点击元素
func BrowserClick(selector string) ToolResult {
	return safeBrowserOp(func(page playwright.Page) ToolResult {
		if err := page.Locator(selector).Click(); err != nil {
			return ToolResult{Output: fmt.Sprintf("点击失败: %v", err), Success: false}
		}
		time.Sleep(500 * time.Millisecond)
		return ToolResult{Output: fmt.Sprintf("已点击: %s", selector), Success: true}
	})
}

// BrowserFill 填写输入框
func BrowserFill(selector, value string) ToolResult {
	return safeBrowserOp(func(page playwright.Page) ToolResult {
		if err := page.Locator(selector).Fill(value); err != nil {
			return ToolResult{Output: fmt.Sprintf("填写失败: %v", err), Success: false}
		}
		return ToolResult{Output: fmt.Sprintf("已填写 %s = %s", selector, value), Success: true}
	})
}

// BrowserSelect 选择下拉项
func BrowserSelect(selector, value string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if _, err := page.Locator(selector).SelectOption(playwright.SelectOptionValues{Values: &[]string{value}}); err != nil {
		return ToolResult{Output: fmt.Sprintf("选择失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("已选择 %s = %s", selector, value), Success: true}
}

// BrowserScreenshot 截图
func BrowserScreenshot(fullPage bool, savePath string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if savePath == "" {
		savePath = filepath.Join(os.TempDir(), fmt.Sprintf("screenshot_%d.png", time.Now().UnixMilli()))
	}
	os.MkdirAll(filepath.Dir(savePath), 0755)

	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(savePath),
		FullPage: playwright.Bool(fullPage),
	}); err != nil {
		return ToolResult{Output: fmt.Sprintf("截图失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("截图已保存: %s", savePath), Success: true}
}

// BrowserGetText 获取页面或元素文本
func BrowserGetText(selector string) ToolResult {
	return safeBrowserOp(func(page playwright.Page) ToolResult {
		var text string
		var err error
		if selector == "" {
			text, err = page.Locator("body").InnerText()
		} else {
			text, err = page.Locator(selector).InnerText()
		}
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("获取文本失败: %v", err), Success: false}
		}
		if len(text) > 8000 {
			text = text[:8000] + "\n... (截断)"
		}
		return ToolResult{Output: text, Success: true}
	})
}

// BrowserGetHTML 获取页面或元素 HTML
func BrowserGetHTML(selector string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	var html string
	if selector == "" {
		html, err = page.Content()
	} else {
		html, err = page.Locator(selector).InnerHTML()
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("获取 HTML 失败: %v", err), Success: false}
	}

	if len(html) > 8000 {
		html = html[:8000] + "\n... (截断)"
	}
	return ToolResult{Output: html, Success: true}
}

// BrowserEvaluate 执行 JavaScript
func BrowserEvaluate(expression string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	result, err := page.Evaluate(expression)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("JS 执行失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("%v", result), Success: true}
}

// BrowserHover 悬停
func BrowserHover(selector string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if err := page.Locator(selector).Hover(); err != nil {
		return ToolResult{Output: fmt.Sprintf("悬停失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("已悬停: %s", selector), Success: true}
}

// BrowserPressKey 按键
func BrowserPressKey(key string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if err := page.Keyboard().Press(key); err != nil {
		return ToolResult{Output: fmt.Sprintf("按键失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("已按键: %s", key), Success: true}
}

// BrowserWait 等待元素
func BrowserWait(selector string, timeout float64) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if timeout <= 0 {
		timeout = 10000
	}

	if err := page.Locator(selector).WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(timeout),
	}); err != nil {
		return ToolResult{Output: fmt.Sprintf("等待超时: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("元素已出现: %s", selector), Success: true}
}

// BrowserGoBack 后退
func BrowserGoBack() ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if _, err := page.GoBack(); err != nil {
		return ToolResult{Output: fmt.Sprintf("后退失败: %v", err), Success: false}
	}
	title, _ := page.Title()
	return ToolResult{Output: fmt.Sprintf("已后退，当前: %s", title), Success: true}
}

// BrowserGoForward 前进
func BrowserGoForward() ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if _, err := page.GoForward(); err != nil {
		return ToolResult{Output: fmt.Sprintf("前进失败: %v", err), Success: false}
	}
	title, _ := page.Title()
	return ToolResult{Output: fmt.Sprintf("已前进，当前: %s", title), Success: true}
}

// BrowserClose 关闭页面
func BrowserClose() ToolResult {
	closePage()
	return ToolResult{Output: "浏览器页面已关闭", Success: true}
}

// BrowserPDF 保存 PDF
func BrowserPDF(savePath string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if savePath == "" {
		savePath = filepath.Join(os.TempDir(), fmt.Sprintf("page_%d.pdf", time.Now().UnixMilli()))
	}
	os.MkdirAll(filepath.Dir(savePath), 0755)

	if _, err := page.PDF(playwright.PagePdfOptions{Path: playwright.String(savePath)}); err != nil {
		return ToolResult{Output: fmt.Sprintf("PDF 生成失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("PDF 已保存: %s", savePath), Success: true}
}

// BrowserUpload 上传文件
func BrowserUpload(selector, filePath string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if err := page.Locator(selector).SetInputFiles(filePath); err != nil {
		return ToolResult{Output: fmt.Sprintf("上传失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("已上传: %s", filePath), Success: true}
}

// ===== 补充功能 =====

// BrowserDrag 拖拽元素
func BrowserDrag(sourceSelector, targetSelector string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	if err := page.DragAndDrop(sourceSelector, targetSelector); err != nil {
		return ToolResult{Output: fmt.Sprintf("拖拽失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("已拖拽 %s → %s", sourceSelector, targetSelector), Success: true}
}

// BrowserIframeClick 在 iframe 内点击元素
func BrowserIframeClick(iframeSelector, selector string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	frame := page.FrameLocator(iframeSelector)
	if err := frame.Locator(selector).Click(); err != nil {
		return ToolResult{Output: fmt.Sprintf("iframe 点击失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("已在 iframe(%s) 中点击 %s", iframeSelector, selector), Success: true}
}

// BrowserIframeFill 在 iframe 内填写
func BrowserIframeFill(iframeSelector, selector, value string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	frame := page.FrameLocator(iframeSelector)
	if err := frame.Locator(selector).Fill(value); err != nil {
		return ToolResult{Output: fmt.Sprintf("iframe 填写失败: %v", err), Success: false}
	}
	return ToolResult{Output: fmt.Sprintf("已在 iframe(%s) 中填写 %s = %s", iframeSelector, selector, value), Success: true}
}

// BrowserClickNewTab 点击元素并切换到新标签页
func BrowserClickNewTab(selector string) ToolResult {
	page, err := getOrCreatePage()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	// 监听新页面打开
	newPage, err := page.ExpectPopup(func() error {
		return page.Locator(selector).Click()
	})
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("点击或新标签打开失败: %v", err), Success: false}
	}

	activePageMu.Lock()
	setupConsoleListener(newPage)
	activePages = append(activePages, newPage)
	activePageMu.Unlock()

	title, _ := newPage.Title()
	url := newPage.URL()
	return ToolResult{Output: fmt.Sprintf("已切换到新标签页\n标题: %s\nURL: %s", title, url), Success: true}
}

// BrowserSwitchTab 切换到指定索引的标签页
func BrowserSwitchTab(index int) ToolResult {
	activePageMu.Lock()
	defer activePageMu.Unlock()

	if index < 0 || index >= len(activePages) {
		return ToolResult{Output: fmt.Sprintf("标签页索引 %d 无效，共 %d 个标签页", index, len(activePages)), Success: false}
	}

	// 把目标页面移到栈顶
	target := activePages[index]
	activePages = append(activePages[:index], activePages[index+1:]...)
	activePages = append(activePages, target)

	title, _ := target.Title()
	return ToolResult{Output: fmt.Sprintf("已切换到标签页 %d: %s", index, title), Success: true}
}

// BrowserListTabs 列出所有标签页
func BrowserListTabs() ToolResult {
	activePageMu.Lock()
	defer activePageMu.Unlock()

	if len(activePages) == 0 {
		return ToolResult{Output: "没有打开的标签页", Success: true}
	}

	var result string
	for i, p := range activePages {
		title, _ := p.Title()
		url := p.URL()
		active := ""
		if i == len(activePages)-1 {
			active = " ← 当前"
		}
		result += fmt.Sprintf("[%d] %s - %s%s\n", i, title, url, active)
	}
	return ToolResult{Output: result, Success: true}
}

// BrowserSetUserAgent 设置自定义 User-Agent
func BrowserSetUserAgent(userAgent string) ToolResult {
	browser, err := ensureHeadedBrowser()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String(userAgent),
	})
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("创建上下文失败: %v", err), Success: false}
	}

	page, err := ctx.NewPage()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("创建页面失败: %v", err), Success: false}
	}

	activePageMu.Lock()
	setupConsoleListener(page)
	activePages = append(activePages, page)
	activePageMu.Unlock()

	return ToolResult{Output: fmt.Sprintf("已设置 User-Agent: %s（新标签页）", userAgent), Success: true}
}

// BrowserConsoleLogs 获取控制台日志
func BrowserConsoleLogs(search string) ToolResult {
	consoleLogsMu.Lock()
	defer consoleLogsMu.Unlock()

	if len(consoleLogs) == 0 {
		return ToolResult{Output: "无控制台日志", Success: true}
	}

	var result string
	for _, log := range consoleLogs {
		if search == "" || strings.Contains(log, search) {
			result += log + "\n"
		}
	}

	if result == "" {
		return ToolResult{Output: fmt.Sprintf("未找到包含 '%s' 的日志", search), Success: true}
	}
	if len(result) > 8000 {
		result = result[:8000] + "\n... (截断)"
	}
	return ToolResult{Output: result, Success: true}
}

// BrowserEmulateDevice 模拟设备
func BrowserEmulateDevice(deviceName string) ToolResult {
	devices := map[string]struct {
		width, height int
		scaleFactor   float64
		userAgent     string
		isMobile      bool
	}{
		"iphone-14":     {390, 844, 3, "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1", true},
		"iphone-14-pro": {393, 852, 3, "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1", true},
		"ipad-pro":      {1024, 1366, 2, "Mozilla/5.0 (iPad; CPU OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1", true},
		"pixel-7":       {412, 915, 2.625, "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Mobile Safari/537.36", true},
		"galaxy-s23":    {360, 780, 3, "Mozilla/5.0 (Linux; Android 13; SM-S911B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Mobile Safari/537.36", true},
		"desktop-1080p": {1920, 1080, 1, "", false},
		"desktop-1440p": {2560, 1440, 1, "", false},
		"laptop":        {1366, 768, 1, "", false},
	}

	dev, ok := devices[deviceName]
	if !ok {
		list := ""
		for k := range devices {
			list += k + ", "
		}
		return ToolResult{Output: fmt.Sprintf("未知设备: %s\n可用设备: %s", deviceName, list), Success: false}
	}

	browser, err := ensureHeadedBrowser()
	if err != nil {
		return ToolResult{Output: err.Error(), Success: false}
	}

	opts := playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: dev.width, Height: dev.height},
		DeviceScaleFactor: playwright.Float(dev.scaleFactor),
		IsMobile:          playwright.Bool(dev.isMobile),
	}
	if dev.userAgent != "" {
		opts.UserAgent = playwright.String(dev.userAgent)
	}

	ctx, err := browser.NewContext(opts)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("创建设备上下文失败: %v", err), Success: false}
	}

	page, err := ctx.NewPage()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("创建页面失败: %v", err), Success: false}
	}

	activePageMu.Lock()
	setupConsoleListener(page)
	activePages = append(activePages, page)
	activePageMu.Unlock()

	return ToolResult{Output: fmt.Sprintf("已模拟设备: %s (%dx%d, %.1fx)", deviceName, dev.width, dev.height, dev.scaleFactor), Success: true}
}
