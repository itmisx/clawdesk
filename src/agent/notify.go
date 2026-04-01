package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// sendNotification 发送通知
func sendNotification(cfg NotifyConfig, taskName, content string) {
	switch cfg.Type {
	case "wecom":
		sendWeComWebhook(cfg.Webhook, taskName, content)
	case "feishu":
		sendFeishuWebhook(cfg.Webhook, taskName, content)
	}
}

// sendWeComWebhook 企业微信 Webhook
// 文档：https://developer.work.weixin.qq.com/document/path/91770
func sendWeComWebhook(webhookURL, taskName, content string) error {
	msg := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": fmt.Sprintf("## 🤖 ClawDesk 定时任务\n**任务**: %s\n**时间**: %s\n\n%s",
				taskName,
				time.Now().Format("2006-01-02 15:04:05"),
				truncate(content, 4000),
			),
		},
	}
	return postJSON(webhookURL, msg)
}

// sendFeishuWebhook 飞书 Webhook
// 文档：https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot
func sendFeishuWebhook(webhookURL, taskName, content string) error {
	msg := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title": map[string]string{
					"tag":     "plain_text",
					"content": "🤖 ClawDesk 定时任务: " + taskName,
				},
				"template": "blue",
			},
			"elements": []map[string]any{
				{
					"tag": "div",
					"text": map[string]string{
						"tag":     "lark_md",
						"content": fmt.Sprintf("**时间**: %s\n\n%s",
							time.Now().Format("2006-01-02 15:04:05"),
							truncate(content, 4000),
						),
					},
				},
			},
		},
	}
	return postJSON(webhookURL, msg)
}

// postJSON 发送 JSON POST 请求
func postJSON(url string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook 返回 HTTP %d", resp.StatusCode)
	}
	return nil
}

// truncate 截断文本
func truncate(s string, maxLen int) string {
	r := []rune(s)
	if len(r) > maxLen {
		return string(r[:maxLen]) + "..."
	}
	return s
}
