package dingtalk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"chatclaw/internal/eino/tools"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/i18n"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

func selectDesc(eng, zh string) string {
	if i18n.GetLocale() == i18n.LocaleZhCN {
		return zh
	}
	return eng
}

// DingTalkSenderConfig configures the dingtalk_sender tool.
type DingTalkSenderConfig struct {
	Gateway          *channels.Gateway
	DefaultChannelID int64  // Auto-filled from channel source context (0 = not set)
	DefaultTargetID  string // Auto-filled from channel source context ("" = not set)
}

// NewDingTalkSenderTool creates a tool that sends messages via a connected DingTalk channel.
func NewDingTalkSenderTool(config *DingTalkSenderConfig) (tool.BaseTool, error) {
	if config == nil || config.Gateway == nil {
		return nil, fmt.Errorf("Gateway is required for dingtalk_sender tool")
	}
	return &dingTalkSenderTool{
		gateway:          config.Gateway,
		defaultChannelID: config.DefaultChannelID,
		defaultTargetID:  config.DefaultTargetID,
	}, nil
}

type dingTalkSenderTool struct {
	gateway          *channels.Gateway
	defaultChannelID int64
	defaultTargetID  string
}

type dingTalkSenderInput struct {
	ChannelID int64  `json:"channel_id"`
	TargetID  string `json:"target_id"`
	Content   string `json:"content"`
	PicURL    string `json:"pic_url"`  // public HTTPS URL for image messages
}

func (t *dingTalkSenderTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	descEN := "Send a message or image to DingTalk via a connected channel. " +
		"Supports sending to group conversations or individual users by conversationId. " +
		"For text: provide content as plain text (sent as Markdown) or JSON with msg_type (text/markdown). " +
		"For images: provide pic_url with a public HTTPS image URL (sent as sampleImageMsg)."
	descZH := "通过已连接的钉钉渠道发送消息或图片。支持发送到群聊或个人会话（通过 conversationId）。" +
		"发送文本：content 可以是纯文本（自动以 Markdown 格式发送）或包含 msg_type（text/markdown）的 JSON。" +
		"发送图片：提供 pic_url（公开 HTTPS 图片地址），以 sampleImageMsg 类型发送。"

	channelIDDescEN := "The channel ID of the connected DingTalk channel to use for sending."
	channelIDDescZH := "用于发送的已连接钉钉渠道 ID。"
	targetIDDescEN := "DingTalk conversationId (the conversation/group ID from the incoming message)."
	targetIDDescZH := "钉钉会话 ID（conversationId），来自收到消息的群聊或单聊 ID。"

	channelIDRequired := true
	targetIDRequired := true

	if t.defaultChannelID > 0 && t.defaultTargetID != "" {
		descEN += fmt.Sprintf(" When this conversation originates from a DingTalk channel, channel_id and target_id are auto-detected (defaults: channel_id=%d, target_id=%s) and can be omitted.", t.defaultChannelID, t.defaultTargetID)
		descZH += fmt.Sprintf(" 当本会话来源于钉钉渠道时，channel_id 和 target_id 已自动检测（默认值：channel_id=%d, target_id=%s），可省略不填。", t.defaultChannelID, t.defaultTargetID)
		channelIDDescEN += fmt.Sprintf(" Auto-detected default: %d. Can be omitted.", t.defaultChannelID)
		channelIDDescZH += fmt.Sprintf(" 已自动检测，默认值：%d，可省略。", t.defaultChannelID)
		targetIDDescEN += fmt.Sprintf(" Auto-detected default: %s. Can be omitted.", t.defaultTargetID)
		targetIDDescZH += fmt.Sprintf(" 已自动检测，默认值：%s，可省略。", t.defaultTargetID)
		channelIDRequired = false
		targetIDRequired = false
	}

	return &schema.ToolInfo{
		Name: tools.ToolIDDingTalkSender,
		Desc: selectDesc(descEN, descZH),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"channel_id": {
				Type:     schema.Integer,
				Desc:     selectDesc(channelIDDescEN, channelIDDescZH),
				Required: channelIDRequired,
			},
			"target_id": {
				Type:     schema.String,
				Desc:     selectDesc(targetIDDescEN, targetIDDescZH),
				Required: targetIDRequired,
			},
			"content": {
				Type: schema.String,
				Desc: selectDesc(
					"Message content. Plain text is sent as Markdown. For structured messages use JSON: {\"msg_type\":\"text\",\"text\":\"hello\"} or {\"msg_type\":\"markdown\",\"title\":\"Title\",\"markdown\":\"## Hello\"}. Not required when pic_url is provided.",
					"消息内容。纯文本以 Markdown 格式发送。结构化消息请用 JSON：{\"msg_type\":\"text\",\"text\":\"hello\"} 或 {\"msg_type\":\"markdown\",\"title\":\"标题\",\"markdown\":\"## 你好\"}。提供 pic_url 时可省略。",
				),
			},
			"pic_url": {
				Type: schema.String,
				Desc: selectDesc(
					"Public HTTPS URL of an image to send as a sampleImageMsg. When provided, content is ignored and the image is sent directly. Must be a valid public URL accessible by DingTalk servers.",
					"要发送的图片公开 HTTPS 地址，以 sampleImageMsg 类型发送。提供此参数时 content 被忽略。必须是钉钉服务器可访问的公开 URL。",
				),
			},
		}),
	}, nil
}

func (t *dingTalkSenderTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in dingTalkSenderInput
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	if in.ChannelID <= 0 && t.defaultChannelID > 0 {
		in.ChannelID = t.defaultChannelID
	}
	if strings.TrimSpace(in.TargetID) == "" && t.defaultTargetID != "" {
		in.TargetID = t.defaultTargetID
	}

	if in.ChannelID <= 0 {
		return "Error: channel_id is required and must be positive", nil
	}
	if strings.TrimSpace(in.TargetID) == "" {
		return "Error: target_id is required", nil
	}

	hasPicURL := strings.TrimSpace(in.PicURL) != ""
	hasContent := strings.TrimSpace(in.Content) != ""
	if !hasPicURL && !hasContent {
		return "Error: either content or pic_url is required", nil
	}

	adapter := t.gateway.GetAdapter(in.ChannelID)
	if adapter == nil {
		return fmt.Sprintf("Error: channel %d is not connected", in.ChannelID), nil
	}
	if adapter.Platform() != channels.PlatformDingTalk {
		return fmt.Sprintf("Error: channel %d is not a DingTalk channel (platform: %s)", in.ChannelID, adapter.Platform()), nil
	}

	// Image by public URL: wrap as JSON so buildDingTalkOutgoingMessage routes correctly
	payload := in.Content
	if hasPicURL {
		imageJSON, _ := json.Marshal(map[string]string{
			"msg_type": "image",
			"pic_url":  strings.TrimSpace(in.PicURL),
		})
		payload = string(imageJSON)
	}

	if err := adapter.SendMessage(ctx, in.TargetID, payload); err != nil {
		return fmt.Sprintf("Error: failed to send message: %s", err.Error()), nil
	}
	if hasPicURL {
		return fmt.Sprintf("Image sent successfully to %s via DingTalk channel %d.", in.TargetID, in.ChannelID), nil
	}
	return fmt.Sprintf("Message sent successfully to %s via DingTalk channel %d.", in.TargetID, in.ChannelID), nil
}
