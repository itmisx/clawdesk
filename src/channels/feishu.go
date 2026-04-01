// 飞书渠道对接
//
// 参考文档：
//   - 飞书开放平台: https://open.feishu.cn/document/home/index
//   - WebSocket 长连接订阅事件: https://open.feishu.cn/document/server-docs/event-subscription-guide/long-connection-based-on-websocket
//   - 接收消息事件 im.message.receive_v1: https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message/events/receive
//   - 发送消息 API: https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message/create
//   - 回复消息 API: https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message/reply
//   - 表情回复(Reaction): https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message-reaction/create
//   - Emoji 类型列表: https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message-reaction/emojis-introduce
//   - Go SDK: https://github.com/larksuite/oapi-sdk-go
//   - OpenClaw 飞书对接参考: https://docs.openclaw.ai/zh-CN/channels/feishu
//
// 认证方式：AppID + AppSecret → tenant_access_token（SDK 自动管理）
// 消息接收：WebSocket 长连接，SDK 自带自动重连和心跳
// 消息回复：Reply API（引用原消息）
// 主动推送：Create API（通过 open_id 发私聊），open_id 收到消息时自动记录并持久化

package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"clawdesk/src/config"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// feishuMsgDedup 飞书消息去重（防止重试投递导致重复回复）
var (
	feishuProcessed   = make(map[string]bool)
	feishuProcessedMu sync.Mutex
)

// saveFeishuOpenID 持久化飞书用户 open_id 到配置
func saveFeishuOpenID(channelID, openID string) {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return
	}
	for i, ch := range cfg.Channels {
		if ch.ID == channelID && ch.Feishu != nil {
			cfg.Channels[i].Feishu.OpenID = openID
			config.Save(cfg)
			return
		}
	}
}

// connectFeishu 使用飞书官方 SDK 建立 WebSocket 长连接，返回主动发消息函数
func connectFeishu(ctx context.Context, cfg config.ChannelConfig, onMessage MessageHandler) (SendFunc, error) {
	appID := cfg.Feishu.AppID
	appSecret := cfg.Feishu.AppSecret

	// 创建 API Client（用于回复和主动发消息）
	apiClient := lark.NewClient(appID, appSecret)

	// 从配置恢复最近用户 open_id（用于主动推送私聊）
	var lastOpenID string
	var openIDMu sync.Mutex
	channelID := cfg.ID

	if cfg.Feishu.OpenID != "" {
		lastOpenID = cfg.Feishu.OpenID
	}

	// 创建事件处理器
	handler := dispatcher.NewEventDispatcher("", "")
	handler.OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		msg := event.Event.Message
		sender := event.Event.Sender


		// 只处理文本消息
		if msg == nil || sender == nil || *msg.MessageType != "text" {
			return nil
		}

		// 消息去重（飞书会重试投递）
		feishuProcessedMu.Lock()
		if feishuProcessed[*msg.MessageId] {
			feishuProcessedMu.Unlock()
			return nil
		}
		feishuProcessed[*msg.MessageId] = true
		// 只保留最近 500 条，防止内存泄漏
		if len(feishuProcessed) > 500 {
			feishuProcessed = make(map[string]bool)
		}
		feishuProcessedMu.Unlock()

		// 解析文本内容
		var textContent struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(*msg.Content), &textContent); err != nil {
			return nil
		}

		// 去掉 @机器人 的文本
		text := strings.TrimSpace(textContent.Text)
		if text == "" {
			return nil
		}

		// 记录用户 open_id 并持久化（用于主动推送）
		if sender.SenderId != nil && sender.SenderId.OpenId != nil {
			oid := *sender.SenderId.OpenId
			openIDMu.Lock()
			changed := lastOpenID != oid
			lastOpenID = oid
			openIDMu.Unlock()
			if changed {
				go saveFeishuOpenID(channelID, oid)
			}
		} else {
		}

		// 异步处理（立即返回，避免飞书 3 秒超时重试）
		messageID := *msg.MessageId
		openID := *sender.SenderId.OpenId

		// 立即在消息上加 emoji reaction（表示正在处理）
		reactResp, _ := apiClient.Im.MessageReaction.Create(ctx, larkim.NewCreateMessageReactionReqBuilder().
			MessageId(messageID).
			Body(larkim.NewCreateMessageReactionReqBodyBuilder().
				ReactionType(larkim.NewEmojiBuilder().EmojiType("Typing").Build()).
				Build()).
			Build())

		// 后台生成真正回复
		go func() {
			if onMessage == nil {
				return
			}
			reply := onMessage(cfg.ID, openID, text)

			// 移除 reaction
			if reactResp != nil && reactResp.Data != nil && reactResp.Data.ReactionId != nil {
				apiClient.Im.MessageReaction.Delete(context.Background(), larkim.NewDeleteMessageReactionReqBuilder().
					MessageId(messageID).
					ReactionId(*reactResp.Data.ReactionId).
					Build())
			}

			if reply != "" {
				replyContent, _ := json.Marshal(map[string]string{"text": reply})
				apiClient.Im.Message.Reply(context.Background(), larkim.NewReplyMessageReqBuilder().
					MessageId(messageID).
					Body(larkim.NewReplyMessageReqBodyBuilder().
						MsgType("text").
						Content(string(replyContent)).
						Build()).
					Build())
			}
		}()
		return nil
	})

	// 创建 WebSocket 客户端
	wsClient := larkws.NewClient(appID, appSecret,
		larkws.WithEventHandler(handler),
		larkws.WithAutoReconnect(true),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	// 后台启动（Start 是阻塞的，放到 goroutine）
	go func() {
		if err := wsClient.Start(ctx); err != nil {
		}
	}()

	// 主动发消息函数（通过 open_id 发私聊）
	sendFunc := func(text string) error {
		openIDMu.Lock()
		openID := lastOpenID
		openIDMu.Unlock()
		if openID == "" {
			return fmt.Errorf("暂无用户记录，请先在飞书给机器人发一条消息")
		}
		content, _ := json.Marshal(map[string]string{"text": text})
		resp, err := apiClient.Im.Message.Create(context.Background(), larkim.NewCreateMessageReqBuilder().
			ReceiveIdType("open_id").
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(openID).
				MsgType("text").
				Content(string(content)).
				Build()).
			Build())
		if err != nil {
			return err
		}
		if resp != nil && resp.Code != 0 {
			return fmt.Errorf("飞书 API 错误(%d): %s", resp.Code, resp.Msg)
		}
		return nil
	}

	return sendFunc, nil
}
