package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// FeishuSenderConfig configures the feishu_sender tool.
type FeishuSenderConfig struct {
	Gateway          *channels.Gateway
	DefaultChannelID int64  // Auto-filled from channel source context (0 = not set)
	DefaultTargetID  string // Auto-filled from channel source context ("" = not set)
}

// NewFeishuSenderTool creates a tool that sends messages via a connected Feishu channel.
// The tool is designed to be passed as an extraTool when a Feishu channel is active.
func NewFeishuSenderTool(config *FeishuSenderConfig) (tool.BaseTool, error) {
	if config == nil || config.Gateway == nil {
		return nil, fmt.Errorf("Gateway is required for feishu_sender tool")
	}
	return &feishuSenderTool{
		gateway:          config.Gateway,
		defaultChannelID: config.DefaultChannelID,
		defaultTargetID:  config.DefaultTargetID,
	}, nil
}

type feishuSenderTool struct {
	gateway          *channels.Gateway
	defaultChannelID int64
	defaultTargetID  string
}

type feishuSenderInput struct {
	ChannelID int64  `json:"channel_id"`
	TargetID  string `json:"target_id"`
	Content   string `json:"content"`
	FilePath  string `json:"file_path"`
	FileType  string `json:"file_type"`
}

var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".bmp": true, ".webp": true,
}

func (t *feishuSenderTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	descEN := "Send a message or file to Feishu (Lark) via a connected channel. " +
		"Supports sending to group chats (oc_xxx) or individual users (ou_xxx). " +
		"For text: provide content as plain text or JSON with msg_type and content fields. " +
		"For files: provide file_path with a local file path; the file will be uploaded to Feishu automatically. " +
		"Images (.jpg/.png/.gif/.bmp/.webp) are sent as image messages; other files as file messages."
	descZH := "通过已连接的飞书渠道发送消息或文件。支持发送到群聊（oc_xxx）或个人用户（ou_xxx）。" +
		"发送文本：content 可以是纯文本或包含 msg_type 和 content 字段的 JSON。" +
		"发送文件：提供 file_path 本地文件路径，工具会自动上传到飞书服务器后发送。" +
		"图片（.jpg/.png/.gif/.bmp/.webp）作为图片消息发送，其他文件作为文件消息发送。"

	channelIDDescEN := "The channel ID of the connected Feishu channel to use for sending."
	channelIDDescZH := "用于发送的已连接飞书渠道 ID。"
	targetIDDescEN := "Feishu receive ID. Use chat ID (oc_xxx) for group chats or open ID (ou_xxx) for direct messages."
	targetIDDescZH := "飞书接收方 ID。群聊使用 chat ID（oc_xxx），私聊使用 open ID（ou_xxx）。"

	channelIDRequired := true
	targetIDRequired := true

	if t.defaultChannelID > 0 && t.defaultTargetID != "" {
		descEN += fmt.Sprintf(" When this conversation originates from a Feishu channel, channel_id and target_id are auto-detected (defaults: channel_id=%d, target_id=%s) and can be omitted.", t.defaultChannelID, t.defaultTargetID)
		descZH += fmt.Sprintf(" 当本会话来源于飞书渠道时，channel_id 和 target_id 已自动检测（默认值：channel_id=%d, target_id=%s），可省略不填。", t.defaultChannelID, t.defaultTargetID)
		channelIDDescEN += fmt.Sprintf(" Auto-detected default: %d. Can be omitted.", t.defaultChannelID)
		channelIDDescZH += fmt.Sprintf(" 已自动检测，默认值：%d，可省略。", t.defaultChannelID)
		targetIDDescEN += fmt.Sprintf(" Auto-detected default: %s. Can be omitted.", t.defaultTargetID)
		targetIDDescZH += fmt.Sprintf(" 已自动检测，默认值：%s，可省略。", t.defaultTargetID)
		channelIDRequired = false
		targetIDRequired = false
	}

	return &schema.ToolInfo{
		Name: tools.ToolIDFeishuSender,
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
					"Message content for text messages. Plain text is auto-wrapped as a text message. For rich messages, use JSON: {\"msg_type\":\"text\",\"content\":{\"text\":\"hello\"}}. Not required when file_path is provided.",
					"文本消息内容。纯文本会自动包装为文本消息。富文本消息请用 JSON：{\"msg_type\":\"text\",\"content\":{\"text\":\"hello\"}}。当提供 file_path 时可不填。",
				),
			},
			"file_path": {
				Type: schema.String,
				Desc: selectDesc(
					"Local file path to upload and send. The file will be uploaded to Feishu server first, then sent as a file or image message. Images (.jpg/.png/.gif/.bmp/.webp) are sent as image messages.",
					"要上传并发送的本地文件路径。文件会先上传到飞书服务器，再作为文件或图片消息发送。图片（.jpg/.png/.gif/.bmp/.webp）作为图片消息发送。",
				),
			},
			"file_type": {
				Type: schema.String,
				Desc: selectDesc(
					"File type for upload. Options: opus, mp4, pdf, doc, xls, ppt, stream. Defaults to auto-detect from extension, falls back to 'stream'. Only used with file_path.",
					"上传文件类型。可选：opus、mp4、pdf、doc、xls、ppt、stream。默认根据扩展名自动检测，兜底为 stream。仅在指定 file_path 时使用。",
				),
			},
		}),
	}, nil
}

func (t *feishuSenderTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in feishuSenderInput
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

	hasContent := strings.TrimSpace(in.Content) != ""
	hasFile := strings.TrimSpace(in.FilePath) != ""
	if !hasContent && !hasFile {
		return "Error: either content or file_path is required", nil
	}

	adapter := t.gateway.GetAdapter(in.ChannelID)
	if adapter == nil {
		return fmt.Sprintf("Error: channel %d is not connected", in.ChannelID), nil
	}
	if adapter.Platform() != channels.PlatformFeishu {
		return fmt.Sprintf("Error: channel %d is not a Feishu channel (platform: %s)", in.ChannelID, adapter.Platform()), nil
	}

	if hasFile {
		return t.handleFileSend(ctx, adapter, in)
	}

	if err := adapter.SendMessage(ctx, in.TargetID, in.Content); err != nil {
		return fmt.Sprintf("Error: failed to send message: %s", err.Error()), nil
	}
	return fmt.Sprintf("Message sent successfully to %s via channel %d.", in.TargetID, in.ChannelID), nil
}

func (t *feishuSenderTool) handleFileSend(ctx context.Context, adapter channels.PlatformAdapter, in feishuSenderInput) (string, error) {
	feishuAdapter, ok := adapter.(*channels.FeishuAdapter)
	if !ok {
		return "Error: file upload is only supported on Feishu channels", nil
	}

	filePath := strings.TrimSpace(in.FilePath)
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Sprintf("Error: file not accessible: %s", err.Error()), nil
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	if imageExtensions[ext] {
		imageKey, err := feishuAdapter.UploadImage(ctx, filePath)
		if err != nil {
			return fmt.Sprintf("Error: failed to upload image: %s", err.Error()), nil
		}

		contentJSON, _ := json.Marshal(map[string]string{"image_key": imageKey})
		msgPayload, _ := json.Marshal(map[string]any{
			"msg_type": "image",
			"content":  json.RawMessage(contentJSON),
		})

		if err := adapter.SendMessage(ctx, in.TargetID, string(msgPayload)); err != nil {
			return fmt.Sprintf("Error: image uploaded (image_key=%s) but failed to send: %s", imageKey, err.Error()), nil
		}
		return fmt.Sprintf("Image sent successfully to %s via channel %d. (image_key=%s)", in.TargetID, in.ChannelID, imageKey), nil
	}

	fileType := inferFileType(in.FileType, ext)
	fileKey, err := feishuAdapter.UploadFile(ctx, filePath, fileType)
	if err != nil {
		return fmt.Sprintf("Error: failed to upload file: %s", err.Error()), nil
	}

	contentJSON, _ := json.Marshal(map[string]string{"file_key": fileKey})
	msgPayload, _ := json.Marshal(map[string]any{
		"msg_type": "file",
		"content":  json.RawMessage(contentJSON),
	})

	if err := adapter.SendMessage(ctx, in.TargetID, string(msgPayload)); err != nil {
		return fmt.Sprintf("Error: file uploaded (file_key=%s) but failed to send: %s", fileKey, err.Error()), nil
	}
	return fmt.Sprintf("File sent successfully to %s via channel %d. (file_key=%s)", in.TargetID, in.ChannelID, fileKey), nil
}

func inferFileType(explicit string, ext string) string {
	if explicit != "" {
		return explicit
	}
	switch ext {
	case ".opus", ".ogg":
		return "opus"
	case ".mp4":
		return "mp4"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx":
		return "doc"
	case ".xls", ".xlsx":
		return "xls"
	case ".ppt", ".pptx":
		return "ppt"
	default:
		return "stream"
	}
}
