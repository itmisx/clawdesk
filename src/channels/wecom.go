// 企业微信 AI Bot 渠道对接
//
// 参考文档：
//   - AI Bot Node SDK: https://www.npmjs.com/package/@wecom/aibot-node-sdk
//   - SDK 源码: https://github.com/WecomTeam/aibot-node-sdk
//   - 智能体长连接: https://developer.work.weixin.qq.com/document/path/101463
//   - 智能体概述: https://developer.work.weixin.qq.com/document/path/104590
//
// 认证方式：BotID + Secret → WebSocket aibot_subscribe 帧认证
// 消息接收：WebSocket 长连接 (wss://openws.work.weixin.qq.com)，cmd=aibot_msg_callback
// 消息回复：WebSocket 帧 cmd=aibot_respond_msg，msgtype=stream（不支持 text 类型）
// 主动推送：WebSocket 帧 cmd=aibot_send_msg，msgtype=markdown（不支持 text 类型）
//           - chatid 参数：单聊填 userid，群聊填 chatid
//           - 支持类型：markdown、template_card、media（file/image/voice/video）
// 心跳保活：每 30 秒 ping/pong

package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"clawdesk/src/config"

	"github.com/gorilla/websocket"
)

const wecomWSURL = "wss://openws.work.weixin.qq.com"

// wecomFrame 企业微信 WebSocket 帧
type wecomFrame struct {
	Cmd     string            `json:"cmd"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body,omitempty"`
	ErrCode int               `json:"errcode,omitempty"`
	ErrMsg  string            `json:"errmsg,omitempty"`
}

// wecomTextMessage 企业微信文本消息
type wecomTextMessage struct {
	MsgID   string `json:"msgid"`
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
	From struct {
		UserID string `json:"userid"`
	} `json:"from"`
	ChatType string `json:"chattype"`
	ChatID   string `json:"chatid"`
}

// wecomConn 企业微信连接（封装 WebSocket 并发写入）
type wecomConn struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (c *wecomConn) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteJSON(v)
}

func (c *wecomConn) readJSON(v any) error {
	return c.ws.ReadJSON(v)
}

func (c *wecomConn) close() {
	c.ws.Close()
}

// saveWecomChatID 持久化企微会话 ID
func saveWecomChatID(channelID, chatID string) {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return
	}
	for i, ch := range cfg.Channels {
		if ch.ID == channelID && ch.Wecom != nil {
			cfg.Channels[i].Wecom.LastChatID = chatID
			config.Save(cfg)
			return
		}
	}
}

// wecomReply 通过 WebSocket 回复消息
func wecomReply(conn *wecomConn, reqID, text string) {
	streamID := fmt.Sprintf("stream_%d", time.Now().UnixMilli())
	replyBody, _ := json.Marshal(map[string]any{
		"msgtype": "stream",
		"stream": map[string]any{
			"id":      streamID,
			"finish":  true,
			"content": text,
		},
	})
	if err := conn.writeJSON(wecomFrame{
		Cmd:     "aibot_respond_msg",
		Headers: map[string]string{"req_id": reqID},
		Body:    replyBody,
	}); err != nil {
	}
}

// wecomSendProactive 通过 WebSocket 主动发送消息（aibot_send_msg）
// 注意：aibot_send_msg 只支持 markdown 和 template_card 类型，不支持 text
func wecomSendProactive(conn *wecomConn, target, chatType, text string) error {
	body, _ := json.Marshal(map[string]any{
		"chatid":  target,
		"msgtype": "markdown",
		"markdown": map[string]string{"content": text},
	})
	err := conn.writeJSON(wecomFrame{
		Cmd:     "aibot_send_msg",
		Headers: map[string]string{"req_id": fmt.Sprintf("send_%d", time.Now().UnixMilli())},
		Body:    body,
	})
	if err != nil {
	}
	return err
}

// connectWecom 建立企业微信 AI Bot WebSocket 长连接
func connectWecom(ctx context.Context, cfg config.ChannelConfig, onMessage MessageHandler, onDisconnect func()) (SendFunc, error) {
	if cfg.Wecom == nil {
		return nil, fmt.Errorf("企业微信配置为空")
	}

	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wecomWSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("WebSocket 连接失败: %w", err)
	}
	conn := &wecomConn{ws: ws}

	// 发送认证帧
	authBody, _ := json.Marshal(map[string]string{"bot_id": cfg.Wecom.BotID, "secret": cfg.Wecom.Secret})
	if err := conn.writeJSON(wecomFrame{
		Cmd:     "aibot_subscribe",
		Headers: map[string]string{"req_id": fmt.Sprintf("auth_%d", time.Now().UnixMilli())},
		Body:    authBody,
	}); err != nil {
		conn.close()
		return nil, fmt.Errorf("发送认证失败: %w", err)
	}

	// 等待认证响应
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	var authResp wecomFrame
	if err := conn.readJSON(&authResp); err != nil {
		conn.close()
		return nil, fmt.Errorf("认证响应读取失败: %w", err)
	}
	ws.SetReadDeadline(time.Time{})

	if authResp.ErrCode != 0 {
		conn.close()
		return nil, fmt.Errorf("认证失败(%d): %s", authResp.ErrCode, authResp.ErrMsg)
	}

	// 从配置恢复最近会话目标
	var lastTarget string
	var lastChatType string = "single"
	var targetMu sync.Mutex
	channelID := cfg.ID
	if cfg.Wecom.LastChatID != "" {
		lastTarget = cfg.Wecom.LastChatID
	}

	go wecomMessageLoop(ctx, conn, cfg, onMessage, onDisconnect, &lastTarget, &lastChatType, &targetMu, channelID)

	// 主动发消息函数（通过同一个 WebSocket 连接）
	sendFn := func(text string) error {
		targetMu.Lock()
		t := lastTarget
		ct := lastChatType
		targetMu.Unlock()
		if t == "" {
			return fmt.Errorf("暂无会话记录，请先在企微给机器人发一条消息")
		}
		return wecomSendProactive(conn, t, ct, text)
	}

	return sendFn, nil
}

// wecomMessageLoop 企业微信消息接收循环
func wecomMessageLoop(ctx context.Context, conn *wecomConn, cfg config.ChannelConfig, onMessage MessageHandler, onDisconnect func(), lastTarget *string, lastChatType *string, targetMu *sync.Mutex, channelID string) {
	defer conn.close()
	defer func() {
		if ctx.Err() == nil && onDisconnect != nil {
			onDisconnect()
		}
	}()

	// 消息去重
	var processedMu sync.Mutex
	processed := make(map[string]bool)

	// Ping 保活（每 30 秒）
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-pingTicker.C:
				if err := conn.writeJSON(wecomFrame{
					Cmd:     "ping",
					Headers: map[string]string{"req_id": fmt.Sprintf("ping_%d", time.Now().UnixMilli())},
				}); err != nil {
					return
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var frame wecomFrame
		if err := conn.readJSON(&frame); err != nil {
			if ctx.Err() != nil {
				return
			}
			return
		}

		switch frame.Cmd {
		case "ping":
			conn.writeJSON(wecomFrame{
				Cmd:     "pong",
				Headers: frame.Headers,
			})

		case "aibot_msg_callback":
			var msg wecomTextMessage
			if err := json.Unmarshal(frame.Body, &msg); err != nil {
				continue
			}
			if msg.MsgType != "text" || msg.Text.Content == "" {
				continue
			}

			// 记录会话目标（单聊用 userid，群聊用 chatid）
			cid := msg.From.UserID
			if msg.ChatID != "" {
				cid = msg.ChatID
			}
			if cid != "" {
				targetMu.Lock()
				if *lastTarget != cid {
					*lastTarget = cid
					go saveWecomChatID(channelID, cid)
				}
				targetMu.Unlock()
			}

			// 去重
			processedMu.Lock()
			if processed[msg.MsgID] {
				processedMu.Unlock()
				continue
			}
			processed[msg.MsgID] = true
			if len(processed) > 500 {
				processed = make(map[string]bool)
			}
			processedMu.Unlock()

			// 异步处理并通过 WebSocket 回复
			reqID := frame.Headers["req_id"]
			userID := msg.From.UserID
			text := msg.Text.Content
			go func() {
				if onMessage == nil {
					return
				}
				reply := onMessage(cfg.ID, userID, text)
				if reply != "" {
					wecomReply(conn, reqID, reply)
				}
			}()

		case "disconnected_event":
			return

		default:
			if frame.Cmd != "" {
			}
		}
	}
}
