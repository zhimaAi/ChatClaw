package channels

import "context"

// MessageHandler processes an incoming message from a platform bot.
type MessageHandler func(msg IncomingMessage)

// IncomingMessage represents a message received from a platform.
type IncomingMessage struct {
	ChannelID  int64  `json:"channel_id"`
	Platform   string `json:"platform"`
	MessageID  string `json:"message_id"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	ChatID     string `json:"chat_id"`
	ChatName   string `json:"chat_name"` // group chat name (if applicable)
	IsGroup    bool   `json:"is_group"`
	Content    string `json:"content"`
	MsgType    string `json:"msg_type"` // text, image, etc.
	RawData    string `json:"raw_data"`
}

// PlatformAdapter abstracts the connection logic for each IM platform.
// Each platform (Feishu, Telegram, etc.) implements this interface.
type PlatformAdapter interface {
	// Platform returns the platform identifier (e.g. "feishu").
	Platform() string

	// Connect establishes a long-lived connection to the platform.
	// configJSON contains platform-specific credentials as a JSON string.
	Connect(ctx context.Context, channelID int64, configJSON string, handler MessageHandler) error

	// Disconnect closes the connection.
	Disconnect(ctx context.Context) error

	// IsConnected returns whether the adapter is currently connected.
	IsConnected() bool

	// SendMessage sends a reply back to the platform.
	SendMessage(ctx context.Context, targetID string, content string) error
}

// AdapterFactory creates a new PlatformAdapter instance for a given platform.
type AdapterFactory func() PlatformAdapter

// registry holds platform adapter factories.
var registry = map[string]AdapterFactory{}

// RegisterAdapter registers an adapter factory for a platform.
func RegisterAdapter(platform string, factory AdapterFactory) {
	registry[platform] = factory
}

// NewAdapter creates a new adapter for the given platform.
// Returns nil if the platform is not registered.
func NewAdapter(platform string) PlatformAdapter {
	factory, ok := registry[platform]
	if !ok {
		return nil
	}
	return factory()
}

// RegisteredPlatforms returns a list of platforms with registered adapters.
func RegisteredPlatforms() []string {
	platforms := make([]string, 0, len(registry))
	for p := range registry {
		platforms = append(platforms, p)
	}
	return platforms
}
