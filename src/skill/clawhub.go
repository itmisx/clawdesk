package skill

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clawdesk/src/config"

	"github.com/playwright-community/playwright-go"
)

// ClawHubSkill 搜索结果
type ClawHubSkill struct {
	Name string `json:"name"`
	Href string `json:"href"`
	Desc string `json:"desc"`
}

// SearchClawHub 通过 playwright 在 clawhub.ai 搜索技能
func SearchClawHub(query string) ([]ClawHubSkill, error) {
	browser, err := ensureBrowser()
	if err != nil {
		return nil, err
	}

	page, err := browser.NewPage()
	if err != nil {
		return nil, fmt.Errorf("创建页面失败: %w", err)
	}
	defer page.Close()

	// 访问技能页
	if _, err := page.Goto("https://clawhub.ai/skills?sort=downloads", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return nil, fmt.Errorf("访问 clawhub.ai 失败: %w", err)
	}

	// 找到搜索框并输入关键词
	input := page.Locator("input.skills-search-input")
	if err := input.Fill(query); err != nil {
		return nil, fmt.Errorf("填写搜索框失败: %w", err)
	}

	// 等待搜索结果刷新
	time.Sleep(2 * time.Second)
	page.Locator("a.skills-table-row").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	})

	// 提取所有结果行（每行是 <a class="skills-table-row" href="...">）
	rows, err := page.Locator("a.skills-table-row").All()
	if err != nil {
		return nil, fmt.Errorf("获取搜索结果失败: %w", err)
	}

	var results []ClawHubSkill
	for _, row := range rows {
		href, _ := row.GetAttribute("href")

		// 名称在 .skills-table-name 下的 span
		name := ""
		nameEl := row.Locator(".skills-table-name span")
		if n, err := nameEl.First().InnerText(); err == nil {
			name = strings.TrimSpace(n)
		}

		// 描述在 .skills-table-summary
		desc := ""
		if d, err := row.Locator(".skills-table-summary").InnerText(); err == nil {
			desc = strings.TrimSpace(d)
		}

		if name != "" && href != "" {
			results = append(results, ClawHubSkill{
				Name: name,
				Href: href,
				Desc: desc,
			})
		}
	}

	return results, nil
}

// InstallClawHubSkill 从 clawhub.ai 下载并安装技能，返回安装的技能名
// href 格式如 "/steipete/weather"，从中提取 slug 直接 HTTP 下载 zip
func InstallClawHubSkill(href string) (string, error) {
	// 从 href 提取 slug（最后一段路径）
	href = strings.TrimPrefix(href, "https://clawhub.ai")
	href = strings.TrimPrefix(href, "/")
	parts := strings.Split(href, "/")
	slug := parts[len(parts)-1]
	if slug == "" {
		return "", fmt.Errorf("无法从 href 提取技能名: %s", href)
	}

	// 直接通过 API 下载 zip（不需要 playwright）
	downloadURL := "https://wry-manatee-359.convex.site/api/v1/download?slug=" + slug
	tmpFile := filepath.Join(os.TempDir(), "clawhub_skill.zip")
	if err := downloadFile(downloadURL, tmpFile); err != nil {
		return "", fmt.Errorf("下载失败: %w", err)
	}
	defer os.Remove(tmpFile)

	return installFromZip(tmpFile)
}

// downloadFile 通过 HTTP 下载文件
func downloadFile(url, destPath string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// installFromZip 解压 zip 并安装到 skills 目录
// 支持两种 zip 结构：
//  1. 扁平结构（文件直接在顶层：SKILL.md, _meta.json）→ 从 _meta.json 的 slug 取目录名
//  2. 带根目录（skillname-version/SKILL.md）→ 去掉版本号作为目录名
func installFromZip(zipPath string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer r.Close()

	if len(r.File) == 0 {
		return "", fmt.Errorf("zip 文件为空")
	}

	// 判断是否扁平结构（第一个文件没有 "/" 前缀）
	flat := true
	rootDir := ""
	if len(r.File) > 0 && strings.Contains(r.File[0].Name, "/") {
		parts := strings.SplitN(r.File[0].Name, "/", 2)
		rootDir = parts[0]
		flat = false
	}

	// 先解压到临时目录，再确定技能名
	tmpDir := filepath.Join(os.TempDir(), "clawhub_extract")
	os.RemoveAll(tmpDir)
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		targetPath := filepath.Join(tmpDir, f.Name)
		os.MkdirAll(filepath.Dir(targetPath), 0755)

		outFile, err := os.Create(targetPath)
		if err != nil {
			return "", fmt.Errorf("创建文件失败 %s: %w", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return "", fmt.Errorf("读取文件失败 %s: %w", f.Name, err)
		}
		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return "", fmt.Errorf("写入文件失败 %s: %w", f.Name, err)
		}
	}

	// 确定技能名和源文件目录
	var skillName string
	var srcDir string

	if flat {
		srcDir = tmpDir
	} else {
		srcDir = filepath.Join(tmpDir, rootDir)
	}

	// 优先从 _meta.json 的 slug 获取名称
	metaPath := filepath.Join(srcDir, "_meta.json")
	if metaData, err := os.ReadFile(metaPath); err == nil {
		var meta struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(metaData, &meta) == nil && meta.Slug != "" {
			skillName = meta.Slug
		}
	}

	// 其次从 SKILL.md frontmatter 获取
	if skillName == "" {
		mdPath := filepath.Join(srcDir, "SKILL.md")
		if mdData, err := os.ReadFile(mdPath); err == nil {
			fm, _ := splitFrontmatter(string(mdData))
			if fm["name"] != "" {
				skillName = fm["name"]
			}
		}
	}

	// 最后用根目录名去掉版本号
	if skillName == "" && rootDir != "" {
		skillName = rootDir
		if idx := strings.LastIndex(rootDir, "-"); idx > 0 {
			suffix := rootDir[idx+1:]
			if len(suffix) > 0 && suffix[0] >= '0' && suffix[0] <= '9' {
				skillName = rootDir[:idx]
			}
		}
	}

	if skillName == "" {
		return "", fmt.Errorf("无法确定技能名称")
	}

	// 移动到目标目录
	installDir := filepath.Join(config.GetConfigDir(), "skills", skillName)
	os.RemoveAll(installDir)
	if err := os.Rename(srcDir, installDir); err != nil {
		// rename 跨设备可能失败，fallback 到复制
		if err := copyDir(srcDir, installDir); err != nil {
			return "", err
		}
	}

	return skillName, nil
}

// copyDir 递归复制目录
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		os.MkdirAll(filepath.Dir(targetPath), 0755)
		return os.WriteFile(targetPath, data, 0644)
	})
}
