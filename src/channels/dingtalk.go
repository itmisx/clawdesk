// 钉钉 Stream 模式渠道对接
//
// 参考文档：
//   - Stream 模式概述: https://open.dingtalk.com/document/direction/stream-mode-protocol-access-description
//   - 机器人接收消息: https://open.dingtalk.com/document/orgapp/receive-message
//   - 单聊主动发消息: https://open.dingtalk.com/document/orgapp/chatbots-send-one-on-one-chat-messages-in-batches
//   - 获取 access_token: https://open.dingtalk.com/document/orgapp/obtain-the-access_token-of-an-internal-app
//
// 认证方式：ClientID (AppKey) + ClientSecret (AppSecret)
// 消息接收：Stream 模式 WebSocket 长连接（自动心跳 + 断线重连）
// 消息回复：通过消息中的 sessionWebhook 回复
// 主动推送：通过 oToMessages/batchSend API（需 robotCode + access_token）

package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"clawdesk/src/config"

	"github.com/gorilla/websocket"
)

const (
	dingtalkOpenURL  = "https://api.dingtalk.com/v1.0/gateway/connections/open"
	dingtalkSendURL  = "https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend"
	dingtalkTokenURL = "https://api.dingtalk.com/v1.0/oauth2/accessToken"
)

// dingtalkOpenResp Stream 注册响应
type dingtalkOpenResp struct {
	Endpoint string `json:"endpoint"`
	Ticket   string `json:"ticket"`
}

// dingtalkStreamMsg Stream WebSocket 帧
type dingtalkStreamMsg struct {
	SpecVersion string            `json:"specVersion"`
	Type        string            `json:"type"`
	Headers     map[string]string `json:"headers"`
	Data        string            `json:"data"`
}

// dingtalkRobotMsg 机器人收到的消息
type dingtalkRobotMsg struct {
	ConversationID   string `json:"conversationId"`
	ChatbotCorpID    string `json:"chatbotCorpId"`
	ChatbotUserID    string `json:"chatbotUserId"`
	MsgID            string `json:"msgId"`
	SenderNick       string `json:"senderNick"`
	SenderStaffID    string `json:"senderStaffId"`
	SenderCorpID     string `json:"senderCorpId"`
	SessionWebhook   string `json:"sessionWebhook"`
	ConversationType string `json:"conversationType"` // "1" 单聊, "2" 群聊
	Text             struct {
		Content string `json:"content"`
	} `json:"text"`
	MsgType string `json:"msgtype"`
}

// saveDingtalkUserID 持久化钉钉用户 staffId
func saveDingtalkUserID(channelID, userID string) {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return
	}
	for i, ch := range cfg.Channels {
		if ch.ID == channelID && ch.Dingtalk != nil {
			cfg.Channels[i].Dingtalk.LastUserID = userID
			config.Save(cfg)
			return
		}
	}
}

// dingtalkGetAccessToken 获取钉钉 access_token
func dingtalkGetAccessToken(clientID, clientSecret string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"appKey":    clientID,
		"appSecret": clientSecret,
	})
	resp, err := http.Post(dingtalkTokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("请求 access_token 失败: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("解析 access_token 失败: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("获取 access_token 失败: %s", string(raw))
	}
	return result.AccessToken, nil
}

// dingtalkRegisterStream 注册 Stream 连接，获取 WebSocket 端点
func dingtalkRegisterStream(clientID, clientSecret string) (*dingtalkOpenResp, error) {
	body, _ := json.Marshal(map[string]any{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"subscriptions": []map[string]string{
			{"type": "EVENT", "topic": "*"},
			{"type": "CALLBACK", "topic": "/v1.0/im/bot/messages/get"},
		},
	})

	resp, err := http.Post(dingtalkOpenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("注册 Stream 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("注册 Stream 失败(%d): %s", resp.StatusCode, string(raw))
	}

	var result dingtalkOpenResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Stream 响应失败: %w", err)
	}
	if result.Endpoint == "" {
		return nil, fmt.Errorf("Stream 端点为空")
	}
	return &result, nil
}

// dingtalkReplyWebhook 通过 sessionWebhook 回复消息
func dingtalkReplyWebhook(webhook, text string) error {
	body, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": text},
	})
	resp, err := http.Post(webhook, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("钉钉回复失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("钉钉回复错误(%d): %s", resp.StatusCode, string(raw))
	}
	return nil
}

// dingtalkSendProactive 主动发送单聊消息（robotCode 等于 clientId/AppKey）
func dingtalkSendProactive(clientID, clientSecret, userID, text string) error {
	token, err := dingtalkGetAccessToken(clientID, clientSecret)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]any{
		"robotCode":    clientID,
		"userIds":      []string{userID},
		"msgKey":       "sampleText",
		"msgParam":     fmt.Sprintf(`{"content":"%s"}`, text),
	})

	req, _ := http.NewRequest("POST", dingtalkSendURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("钉钉主动发送失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("钉钉主动发送错误(%d): %s", resp.StatusCode, string(raw))
	}
	return nil
}

// connectDingtalk 建立钉钉 Stream 模式长连接
func connectDingtalk(ctx context.Context, cfg config.ChannelConfig, onMessage MessageHandler, onDisconnect func()) (SendFunc, error) {
	if cfg.Dingtalk == nil {
		return nil, fmt.Errorf("钉钉配置为空")
	}

	clientID := cfg.Dingtalk.ClientID
	clientSecret := cfg.Dingtalk.ClientSecret

	// 注册 Stream 获取 WebSocket 端点
	streamResp, err := dingtalkRegisterStream(clientID, clientSecret)
	if err != nil {
		return nil, err
	}

	// 连接 WebSocket
	wsURL := fmt.Sprintf("%s?ticket=%s", streamResp.Endpoint, streamResp.Ticket)
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("钉钉 WebSocket 连接失败: %w", err)
	}

	// 从配置恢复最近用户
	var lastUserID string
	var userMu sync.Mutex
	channelID := cfg.ID
	if cfg.Dingtalk.LastUserID != "" {
		lastUserID = cfg.Dingtalk.LastUserID
	}

	// 消息去重
	var processedMu sync.Mutex
	processed := make(map[string]bool)


	// 消息接收循环
	go func() {
		defer ws.Close()
		defer func() {
			if ctx.Err() == nil && onDisconnect != nil {
				onDisconnect()
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			var msg dingtalkStreamMsg
			if err := ws.ReadJSON(&msg); err != nil {
				if ctx.Err() != nil {
					return
				}
				return
			}

			// 处理系统事件
			switch msg.Type {
			case "SYSTEM":
				// 系统事件（如 ping），回复 ACK
				if topic, ok := msg.Headers["topic"]; ok && topic == "ping" {
					ack := map[string]any{
						"code":    200,
						"headers": msg.Headers,
						"message": "OK",
						"data":    msg.Data,
					}
					ws.WriteJSON(ack)
				}
				continue

			case "CALLBACK":
				// 机器人消息回调
				topic := msg.Headers["topic"]
				messageID := msg.Headers["messageId"]

				// 先回 ACK（钉钉要求 3 秒内响应）
				ack := map[string]any{
					"code":    200,
					"headers": map[string]string{"contentType": "application/json", "messageId": messageID},
					"message": "OK",
					"data":    "{}",
				}
				ws.WriteJSON(ack)

				if topic != "/v1.0/im/bot/messages/get" {
					continue
				}

				// 解析机器人消息
				var robotMsg dingtalkRobotMsg
				if err := json.Unmarshal([]byte(msg.Data), &robotMsg); err != nil {
					continue
				}

				if robotMsg.MsgType != "text" || robotMsg.Text.Content == "" {
					continue
				}

				// 去重
				processedMu.Lock()
				if processed[robotMsg.MsgID] {
					processedMu.Unlock()
					continue
				}
				processed[robotMsg.MsgID] = true
				if len(processed) > 500 {
					processed = make(map[string]bool)
				}
				processedMu.Unlock()

				// 记录用户 ID
				if robotMsg.SenderStaffID != "" {
					userMu.Lock()
					changed := lastUserID != robotMsg.SenderStaffID
					lastUserID = robotMsg.SenderStaffID
					userMu.Unlock()
					if changed {
						go saveDingtalkUserID(channelID, robotMsg.SenderStaffID)
					}
				}

				// 异步处理并回复
				webhook := robotMsg.SessionWebhook
				userID := robotMsg.SenderStaffID
				text := robotMsg.Text.Content
				go func() {
					if onMessage == nil {
						return
					}
					reply := onMessage(cfg.ID, userID, text)
					if reply != "" && webhook != "" {
						if err := dingtalkReplyWebhook(webhook, reply); err != nil {
						}
					}
				}()

			case "EVENT":
				// 事件订阅，回 ACK 即可
				messageID := msg.Headers["messageId"]
				ack := map[string]any{
					"code":    200,
					"headers": map[string]string{"contentType": "application/json", "messageId": messageID},
					"message": "OK",
					"data":    "{}",
				}
				ws.WriteJSON(ack)

			default:
			}
		}
	}()

	// 心跳保活（官方 SDK 使用 WebSocket ping，120 秒间隔）
	go func() {
		ticker := time.NewTicker(120 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// 主动发消息函数（robotCode = clientId）
	sendFn := func(text string) error {
		userMu.Lock()
		uid := lastUserID
		userMu.Unlock()
		if uid == "" {
			return fmt.Errorf("暂无用户记录，请先在钉钉给机器人发一条消息")
		}
		return dingtalkSendProactive(clientID, clientSecret, uid, text)
	}

	return sendFn, nil
}
