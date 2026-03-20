package dingtalk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"chatclaw/internal/eino/tools"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/i18n"
	"chatclaw/internal/services/oss"

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
	PicURL    string `json:"pic_url"`   // public HTTPS URL for image messages
	FilePath  string `json:"file_path"` // local file path for upload + file reply
	SendMode  string `json:"send_mode"` // "typewriter" (default) or "normal"
}

var dingtalkImageExts = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".gif":  {},
	".bmp":  {},
	".webp": {},
}

func isImageFilePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	_, ok := dingtalkImageExts[ext]
	return ok
}

// isNetworkURL returns true if the path looks like an http/https URL.
func isNetworkURL(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

// isImagePath checks whether a path (local or URL) points to an image file.
// For URLs, query strings and fragments are stripped before checking the extension.
func isImagePath(path string) bool {
	p := strings.TrimSpace(path)
	if isNetworkURL(p) {
		if idx := strings.IndexAny(p, "?#"); idx != -1 {
			p = p[:idx]
		}
	}
	return isImageFilePath(p)
}

func (t *dingTalkSenderTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	descEN := "Send a message, image or file to DingTalk via a connected channel. " +
		"Supports sending to group conversations or individual users by conversationId. " +
		"For text: provide content as plain text (sent as Markdown) or JSON with msg_type (text/markdown). " +
		"For images: provide pic_url with a public HTTPS image URL (sent as image message). " +
		"For files: provide file_path; the tool uploads media to DingTalk then sends a file message. " +
		"By default text messages use typewriter mode (animated streaming card). " +
		"Use send_mode=normal for short acknowledgments, simple replies, or when animation is not appropriate."
	descZH := "通过已连接的钉钉渠道发送消息、图片或文件。支持发送到群聊或个人会话（通过 conversationId）。" +
		"发送文本：content 可以是纯文本（自动以 Markdown 格式发送）或包含 msg_type（text/markdown）的 JSON。" +
		"发送图片：提供 pic_url（公开 HTTPS 图片地址），以图片消息发送。" +
		"发送文件：提供 file_path，本工具会先上传到钉钉再发送文件消息。" +
		"文本消息默认使用打字机模式（动画流式卡片）。" +
		"对于简短回复、确认消息或不适合动画的场景，使用 send_mode=normal 发送普通消息。"

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
					"消息内容。纯文本以 Markdown 格式发送。结构化消息请用 JSON：{\"msg_type\":\"text\",\"text\":\"hello\"} 或 {\"msg_type\":\"markdown\",\"title\":\"标题\",\"markdown\":\"## 你好\"}。提供 pic_url 或 file_path 时可省略。",
				),
			},
			"pic_url": {
				Type: schema.String,
				Desc: selectDesc(
					"Public HTTPS URL of an image to send as image message. When provided, content is ignored and the image is sent directly. Must be a valid public URL accessible by DingTalk servers.",
					"要发送的图片公开 HTTPS 地址，以图片消息发送。提供此参数时 content 被忽略。必须是钉钉服务器可访问的公开 URL。",
				),
			},
			"file_path": {
				Type: schema.String,
				Desc: selectDesc(
					"Local file path. Images are uploaded to OSS first, then sent as Markdown. Non-image files trigger a notice (DingTalk only supports image attachments).",
					"本地文件路径。图片会上传至 OSS 后以 Markdown 发送。非图片文件会发送提示（钉钉仅支持图片附件）。",
				),
			},
			"send_mode": {
				Type: schema.String,
				Desc: selectDesc(
					"Message send mode. 'typewriter' (default): animated streaming card that reveals text progressively — ideal for AI-generated long replies. 'normal': instant webhook message — use for short acknowledgments, one-word answers, images, or files.",
					"消息发送模式。'typewriter'（默认）：打字机动画流式卡片，逐步展示文字，适合 AI 生成的长篇回复。'normal'：即时 webhook 消息，适用于简短确认、单字回复、图片或文件。",
				),
			},
		}),
	}, nil
}

func (t *dingTalkSenderTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	in, err := t.parseAndResolveInput(argsJSON)
	if err != nil {
		return "", err
	}
	if msg, ok := validateDingTalkSenderInput(in); !ok {
		return msg, nil
	}

	adapter, msg := t.resolveDingTalkAdapter(in.ChannelID)
	if msg != "" {
		return msg, nil
	}

	// Priority order:
	// 1) When file_path or pic_url is set: handle via sendByFilePath.
	//    - Network URL: embed directly in Markdown as ![image](url).
	//    - Local path: upload to OSS first, then embed in Markdown.
	// 2) When send_mode is "typewriter" and content is text-only, use streaming card.
	// 3) Otherwise send content as text/markdown via webhook.
	if in.FilePath != "" {
		return t.sendByFilePath(ctx, adapter, in)
	}
	if in.PicURL != "" {
		in2 := in
		in2.FilePath = in.PicURL
		return t.sendByFilePath(ctx, adapter, in2)
	}
	if in.SendMode == "typewriter" {
		return t.sendTypewriter(ctx, adapter, in)
	}
	return t.sendTextOrMarkdown(ctx, adapter, in)
}

func (t *dingTalkSenderTool) parseAndResolveInput(argsJSON string) (dingTalkSenderInput, error) {
	var in dingTalkSenderInput
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		slog.Error("[dingtalk_sender] failed to parse arguments",
			"error", err,
			"args_json", argsJSON,
		)
		return dingTalkSenderInput{}, fmt.Errorf("parse arguments: %w", err)
	}

	if in.ChannelID <= 0 && t.defaultChannelID > 0 {
		in.ChannelID = t.defaultChannelID
	}
	if strings.TrimSpace(in.TargetID) == "" && t.defaultTargetID != "" {
		in.TargetID = t.defaultTargetID
	}

	in.TargetID = strings.TrimSpace(in.TargetID)
	in.PicURL = strings.TrimSpace(in.PicURL)
	in.FilePath = strings.TrimSpace(in.FilePath)
	in.SendMode = strings.ToLower(strings.TrimSpace(in.SendMode))
	if in.SendMode == "" {
		in.SendMode = "typewriter"
	}

	return in, nil
}

func validateDingTalkSenderInput(in dingTalkSenderInput) (string, bool) {
	if in.ChannelID <= 0 {
		slog.Warn("[dingtalk_sender] validation failed",
			"reason", "channel_id is required and must be positive",
		)
		return "Error: channel_id is required and must be positive", false
	}
	if in.TargetID == "" {
		slog.Warn("[dingtalk_sender] validation failed",
			"reason", "target_id is required",
			"channel_id", in.ChannelID,
		)
		return "Error: target_id is required", false
	}
	hasPicURL := in.PicURL != ""
	hasFilePath := in.FilePath != ""
	hasContent := strings.TrimSpace(in.Content) != ""
	if !hasPicURL && !hasFilePath && !hasContent {
		slog.Warn("[dingtalk_sender] validation failed",
			"reason", "either content, pic_url or file_path is required",
			"channel_id", in.ChannelID,
			"target_id", in.TargetID,
		)
		return "Error: either content, pic_url or file_path is required", false
	}
	return "", true
}

func (t *dingTalkSenderTool) resolveDingTalkAdapter(channelID int64) (channels.PlatformAdapter, string) {
	adapter := t.gateway.GetAdapter(channelID)
	if adapter == nil {
		slog.Error("[dingtalk_sender] channel adapter not connected",
			"channel_id", channelID,
		)
		return nil, fmt.Sprintf("Error: channel %d is not connected", channelID)
	}
	if adapter.Platform() != channels.PlatformDingTalk {
		slog.Error("[dingtalk_sender] channel platform mismatch",
			"channel_id", channelID,
			"platform", adapter.Platform(),
		)
		return nil, fmt.Sprintf("Error: channel %d is not a DingTalk channel (platform: %s)", channelID, adapter.Platform())
	}
	return adapter, ""
}

// sendByFilePath handles the file_path field (also used for pic_url):
//   - Non-image files: DingTalk does not support them; send an explanatory notice message.
//   - Image files that are already a network URL: send directly as a Markdown message.
//   - Local image files: upload to ChatClaw OSS first, then send as a Markdown message.
func (t *dingTalkSenderTool) sendByFilePath(ctx context.Context, adapter channels.PlatformAdapter, in dingTalkSenderInput) (string, error) {
	filePath := in.FilePath

	// DingTalk channels only support image attachments.
	// Send an explanatory notice for any other file type.
	if !isImagePath(filePath) {
		fileName := filepath.Base(filePath)
		ext := strings.ToLower(filepath.Ext(fileName))
		if ext == "" {
			ext = "unknown"
		}
		notice := fmt.Sprintf(
			"**[文件通知]** 文件「%s」（格式：%s）无法通过钉钉直接发送，钉钉频道仅支持图片类型附件（jpg / png / gif 等）。",
			fileName, ext,
		)
		noticePayload, err := json.Marshal(map[string]string{
			"msg_type": "markdown",
			"title":    "文件通知",
			"markdown": notice,
		})
		if err != nil {
			slog.Error("[dingtalk_sender] failed to marshal non-image file notice",
				"file_path", filePath,
				"error", err,
			)
			return fmt.Sprintf("Error: failed to build file notice payload: %s", err.Error()), nil
		}
		if err := sendDingTalkPayload(ctx, adapter, in.ChannelID, in.TargetID, string(noticePayload)); err != nil {
			return fmt.Sprintf("Error: failed to send file notice: %s", err.Error()), nil
		}
		slog.Info("[dingtalk_sender] non-image file notice sent",
			"channel_id", in.ChannelID,
			"target_id", in.TargetID,
			"file_path", filePath,
		)
		return fmt.Sprintf("File type notice sent to %s: DingTalk only supports image attachments, file type %s is not supported.", in.TargetID, ext), nil
	}

	// Resolve the public image URL: upload local files to OSS, pass through network URLs.
	imageURL := filePath
	if !isNetworkURL(filePath) {
		slog.Info("[dingtalk_sender] uploading local image to OSS",
			"channel_id", in.ChannelID,
			"file_path", filePath,
		)
		uploaded, err := oss.UploadImage(ctx, filePath)
		if err != nil {
			slog.Error("[dingtalk_sender] failed to upload image to OSS",
				"file_path", filePath,
				"error", err,
			)
			return fmt.Sprintf("Error: failed to upload image to OSS: %s", err.Error()), nil
		}
		imageURL = uploaded
		slog.Info("[dingtalk_sender] image uploaded to OSS",
			"channel_id", in.ChannelID,
			"image_url", imageURL,
		)
	}

	// Build Markdown content: optional text followed by the embedded image.
	markdownContent := fmt.Sprintf("![image](%s)", imageURL)
	title := "图片"
	if strings.TrimSpace(in.Content) != "" {
		markdownContent = in.Content + "\n\n" + markdownContent
		firstLine := strings.SplitN(strings.TrimSpace(in.Content), "\n", 2)[0]
		if len([]rune(firstLine)) > 20 {
			firstLine = string([]rune(firstLine)[:20]) + "…"
		}
		title = firstLine
	}

	payloadBytes, err := json.Marshal(map[string]string{
		"msg_type": "markdown",
		"title":    title,
		"markdown": markdownContent,
	})
	if err != nil {
		slog.Error("[dingtalk_sender] failed to marshal image markdown payload",
			"error", err,
			"image_url", imageURL,
			"target_id", in.TargetID,
		)
		return fmt.Sprintf("Error: failed to build image message payload: %s", err.Error()), nil
	}
	slog.Info("[dingtalk_sender] sending image markdown message",
		"channel_id", in.ChannelID,
		"target_id", in.TargetID,
		"payload", string(payloadBytes),
	)
	if err := sendDingTalkPayload(ctx, adapter, in.ChannelID, in.TargetID, string(payloadBytes)); err != nil {
		return fmt.Sprintf("Error: failed to send image message: %s", err.Error()), nil
	}

	return fmt.Sprintf("Image sent successfully to %s via DingTalk channel %d.", in.TargetID, in.ChannelID), nil
}

func (t *dingTalkSenderTool) sendTextOrMarkdown(ctx context.Context, adapter channels.PlatformAdapter, in dingTalkSenderInput) (string, error) {
	if err := sendDingTalkPayload(ctx, adapter, in.ChannelID, in.TargetID, in.Content); err != nil {
		return fmt.Sprintf("Error: failed to send message: %s", err.Error()), nil
	}
	return fmt.Sprintf("Message sent successfully to %s via DingTalk channel %d.", in.TargetID, in.ChannelID), nil
}

// sendTypewriter sends a text message using the streaming interactive card (typewriter mode).
// It falls back to normal webhook delivery if the adapter is not a DingTalkAdapter or if
// the conversation metadata (type/staffId) is not yet cached.
func (t *dingTalkSenderTool) sendTypewriter(ctx context.Context, adapter channels.PlatformAdapter, in dingTalkSenderInput) (string, error) {
	dingAdapter, ok := adapter.(*channels.DingTalkAdapter)
	if !ok {
		slog.Warn("[dingtalk_sender] typewriter mode requires DingTalkAdapter, falling back to normal",
			"channel_id", in.ChannelID,
		)
		return t.sendTextOrMarkdown(ctx, adapter, in)
	}

	if err := dingAdapter.SendStreamingCard(ctx, in.TargetID, in.Content); err != nil {
		slog.Error("[dingtalk_sender] typewriter send failed, falling back to normal",
			"channel_id", in.ChannelID,
			"target_id", in.TargetID,
			"error", err,
		)
		// Graceful fallback: deliver via webhook so the message is never lost.
		return t.sendTextOrMarkdown(ctx, adapter, in)
	}
	return fmt.Sprintf("Message sent successfully (typewriter mode) to %s via DingTalk channel %d.", in.TargetID, in.ChannelID), nil
}

func sendDingTalkPayload(ctx context.Context, adapter channels.PlatformAdapter, channelID int64, targetID string, payload string) error {
	if err := adapter.SendMessage(ctx, targetID, payload); err != nil {
		slog.Error("[dingtalk_sender] failed to send message",
			"channel_id", channelID,
			"target_id", targetID,
			"payload", payload,
			"error", err,
		)
		return err
	}
	return nil
}
