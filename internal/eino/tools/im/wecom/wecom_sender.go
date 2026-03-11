package wecom

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

// WeComSenderConfig configures the wecom_sender tool.
type WeComSenderConfig struct {
	Gateway          *channels.Gateway
	DefaultChannelID int64  // Auto-filled from channel source context (0 = not set)
	DefaultTargetID  string // Auto-filled from channel source context ("" = not set)
}

// NewWeComSenderTool creates a tool that sends messages via a connected WeCom channel.
func NewWeComSenderTool(config *WeComSenderConfig) (tool.BaseTool, error) {
	if config == nil || config.Gateway == nil {
		return nil, fmt.Errorf("Gateway is required for wecom_sender tool")
	}
	return &wecomSenderTool{
		gateway:          config.Gateway,
		defaultChannelID: config.DefaultChannelID,
		defaultTargetID:  config.DefaultTargetID,
	}, nil
}

type wecomSenderTool struct {
	gateway          *channels.Gateway
	defaultChannelID int64
	defaultTargetID  string
}

type wecomSenderInput struct {
	ChannelID int64  `json:"channel_id"`
	TargetID  string `json:"target_id"`
	Content   string `json:"content"`
	FilePath  string `json:"file_path"`
	FileURL   string `json:"file_url"`
	ImageURL  string `json:"image_url"`
}

var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".bmp": true, ".webp": true,
}

func (t *wecomSenderTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	descEN := "Send a message or file to WeCom (企业微信) via a connected channel. " +
		"Supports sending to group chats or individual users. " +
		"For text: provide content as plain text or Markdown (WeCom supports Markdown formatting). " +
		"For images: provide image_url with a publicly accessible image URL. " +
		"For files: provide file_url with a publicly accessible file URL, or file_path for local files (will need to be served)."
	descZH := "通过已连接的企业微信渠道发送消息或文件。支持发送到群聊或个人用户。" +
		"发送文本：content 可以是纯文本或 Markdown 格式（企业微信支持 Markdown）。" +
		"发送图片：提供 image_url（可公开访问的图片 URL）。" +
		"发送文件：提供 file_url（可公开访问的文件 URL）或 file_path（本地文件路径，需要能被访问）。"

	channelIDDescEN := "The channel ID of the connected WeCom channel to use for sending."
	channelIDDescZH := "用于发送的已连接企业微信渠道 ID。"
	targetIDDescEN := "WeCom receive ID. Use chatid for group chats or userid for direct messages."
	targetIDDescZH := "企业微信接收方 ID。群聊使用 chatid，私聊使用 userid。"

	channelIDRequired := true
	targetIDRequired := true

	if t.defaultChannelID > 0 && t.defaultTargetID != "" {
		descEN += fmt.Sprintf(" When this conversation originates from a WeCom channel, channel_id and target_id are auto-detected (defaults: channel_id=%d, target_id=%s) and can be omitted.", t.defaultChannelID, t.defaultTargetID)
		descZH += fmt.Sprintf(" 当本会话来源于企业微信渠道时，channel_id 和 target_id 已自动检测（默认值：channel_id=%d, target_id=%s），可省略不填。", t.defaultChannelID, t.defaultTargetID)
		channelIDDescEN += fmt.Sprintf(" Auto-detected default: %d. Can be omitted.", t.defaultChannelID)
		channelIDDescZH += fmt.Sprintf(" 已自动检测，默认值：%d，可省略。", t.defaultChannelID)
		targetIDDescEN += fmt.Sprintf(" Auto-detected default: %s. Can be omitted.", t.defaultTargetID)
		targetIDDescZH += fmt.Sprintf(" 已自动检测，默认值：%s，可省略。", t.defaultTargetID)
		channelIDRequired = false
		targetIDRequired = false
	}

	return &schema.ToolInfo{
		Name: tools.ToolIDWeComSender,
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
					"Message content for text/markdown messages. Supports Markdown formatting. Not required when image_url or file_url is provided.",
					"文本/Markdown消息内容。支持 Markdown 格式。当提供 image_url 或 file_url 时可不填。",
				),
			},
			"image_url": {
				Type: schema.String,
				Desc: selectDesc(
					"Public URL of an image to send. The image will be embedded in a Markdown message.",
					"要发送的图片公开 URL。图片将嵌入到 Markdown 消息中发送。",
				),
			},
			"file_url": {
				Type: schema.String,
				Desc: selectDesc(
					"Public URL of a file to send. The file will be sent as a Markdown link.",
					"要发送的文件公开 URL。文件将作为 Markdown 链接发送。",
				),
			},
			"file_path": {
				Type: schema.String,
				Desc: selectDesc(
					"Local file path. Note: WeCom AI Bot requires files to be accessible via URL, so this may need additional setup.",
					"本地文件路径。注意：企业微信 AI Bot 需要文件可通过 URL 访问，可能需要额外配置。",
				),
			},
		}),
	}, nil
}

func (t *wecomSenderTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in wecomSenderInput
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
	hasImageURL := strings.TrimSpace(in.ImageURL) != ""
	hasFileURL := strings.TrimSpace(in.FileURL) != ""
	hasFilePath := strings.TrimSpace(in.FilePath) != ""

	if !hasContent && !hasImageURL && !hasFileURL && !hasFilePath {
		return "Error: at least one of content, image_url, file_url, or file_path is required", nil
	}

	adapter := t.gateway.GetAdapter(in.ChannelID)
	if adapter == nil {
		return fmt.Sprintf("Error: channel %d is not connected", in.ChannelID), nil
	}
	if adapter.Platform() != channels.PlatformWeCom {
		return fmt.Sprintf("Error: channel %d is not a WeCom channel (platform: %s)", in.ChannelID, adapter.Platform()), nil
	}

	wecomAdapter, ok := adapter.(*channels.WeComAdapter)
	if !ok {
		return "Error: failed to get WeCom adapter instance", nil
	}

	if hasImageURL {
		if err := wecomAdapter.SendImage(ctx, in.TargetID, in.ImageURL); err != nil {
			return fmt.Sprintf("Error: failed to send image: %s", err.Error()), nil
		}
		return fmt.Sprintf("Image sent successfully to %s via channel %d.", in.TargetID, in.ChannelID), nil
	}

	if hasFileURL {
		fileName := filepath.Base(in.FileURL)
		if err := wecomAdapter.SendFile(ctx, in.TargetID, in.FileURL, fileName); err != nil {
			return fmt.Sprintf("Error: failed to send file: %s", err.Error()), nil
		}
		return fmt.Sprintf("File sent successfully to %s via channel %d.", in.TargetID, in.ChannelID), nil
	}

	if hasFilePath {
		filePath := strings.TrimSpace(in.FilePath)
		if _, err := os.Stat(filePath); err != nil {
			return fmt.Sprintf("Error: file not accessible: %s", err.Error()), nil
		}
		return "Error: local file upload is not directly supported by WeCom AI Bot. Please provide a publicly accessible file_url instead.", nil
	}

	if err := adapter.SendMessage(ctx, in.TargetID, in.Content); err != nil {
		return fmt.Sprintf("Error: failed to send message: %s", err.Error()), nil
	}
	return fmt.Sprintf("Message sent successfully to %s via channel %d.", in.TargetID, in.ChannelID), nil
}
