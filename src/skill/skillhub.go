package skill

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"clawdesk/src/config"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// SkillHubSkill 搜索结果中的技能
type SkillHubSkill struct {
	Name          string   `json:"name"`
	Slug          string   `json:"slug"`
	Category      string   `json:"category"`
	Description   string   `json:"description"`
	DescriptionZh string   `json:"description_zh"`
	Version       string   `json:"version"`
	OwnerName     string   `json:"ownerName"`
	Score         float64  `json:"score"`
	Stars         int      `json:"stars"`
	Downloads     int      `json:"downloads"`
	Tags          []string `json:"tags"`
}

var skillHubCLIOnce sync.Once
var skillHubCLIReady bool

// EnsureSkillHubCLI 确保 skillhub CLI 已安装（幂等，不重复安装）
func EnsureSkillHubCLI() error {
	var installErr error
	skillHubCLIOnce.Do(func() {
		if _, err := exec.LookPath("skillhub"); err == nil {
			skillHubCLIReady = true
			return
		}
		cmd := exec.Command("bash", "-c",
			"curl -fsSL https://skillhub-1388575217.cos.ap-guangzhou.myqcloud.com/install/install.sh | bash -s -- --no-skills")
		output, err := cmd.CombinedOutput()
		if err != nil {
			installErr = fmt.Errorf("安装 skillhub CLI 失败: %s\n%s", err.Error(), string(output))
			return
		}
		skillHubCLIReady = true
	})
	return installErr
}

// SearchSkillHub 从 SkillHub 搜索技能
func SearchSkillHub(query string, limit int) ([]SkillHubSkill, error) {
	if limit <= 0 || limit > 24 {
		limit = 10
	}

	u := fmt.Sprintf("https://lightmake.site/api/skills?page=1&pageSize=%d&sortBy=score&order=desc&keyword=%s",
		limit, url.QueryEscape(query))

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ClawDesk/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("搜索请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("搜索失败: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Total  int             `json:"total"`
			Skills []SkillHubSkill `json:"skills"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("搜索失败: %s", result.Message)
	}
	return result.Data.Skills, nil
}

// InstallSkillHubSkill 通过 skillhub CLI 安装技能到 ~/.clawdesk/skills/
func InstallSkillHubSkill(slug string) error {
	if err := EnsureSkillHubCLI(); err != nil {
		return err
	}

	installDir := filepath.Join(config.GetConfigDir(), "skills")
	os.MkdirAll(installDir, 0755)

	// 用 --dir 指定安装到我们的 skills 目录
	cmd := exec.Command("skillhub", "--dir", installDir, "install", slug)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("安装失败: %s\n%s", err.Error(), strings.TrimSpace(string(output)))
	}
	return nil
}
