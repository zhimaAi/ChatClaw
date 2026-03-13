package assistantmcp

// SendMessageInput mirrors the minimum fields needed from chat.SendMessageInput.
type SendMessageInput struct {
	ConversationID int64  `json:"conversation_id"`
	Content        string `json:"content"`
	TabID          string `json:"tab_id"`
}

// SendMessageResult mirrors chat.SendMessageResult.
type SendMessageResult struct {
	RequestID string `json:"request_id"`
	MessageID int64  `json:"message_id"`
}

// ChatBridge is the interface the assistant MCP server uses to invoke AI chat.
// Implemented by chat.ChatService; injected via SetChatService to avoid cyclic imports.
type ChatBridge interface {
	FindOrCreateConversation(agentID int64, externalID, name string) (int64, error)
	SendMessage(input SendMessageInput) (*SendMessageResult, error)
	WaitForGeneration(conversationID int64, requestID string) error
	GetLatestReply(conversationID int64) (string, error)
}

// chatBridgeAdapter wraps concrete service functions as a ChatBridge.
// Since we cannot import chat/conversations, bootstrap wires this via function adapters.
type chatBridgeAdapter struct {
	findOrCreateConvFn func(agentID int64, externalID, name string) (int64, error)
	sendFn             func(convID int64, content, tabID string) (requestID string, messageID int64, err error)
	waitFn             func(convID int64, requestID string) error
	getLatestReplyFn   func(convID int64) (string, error)
}

func (a *chatBridgeAdapter) FindOrCreateConversation(agentID int64, externalID, name string) (int64, error) {
	return a.findOrCreateConvFn(agentID, externalID, name)
}

func (a *chatBridgeAdapter) SendMessage(input SendMessageInput) (*SendMessageResult, error) {
	reqID, msgID, err := a.sendFn(input.ConversationID, input.Content, input.TabID)
	if err != nil {
		return nil, err
	}
	return &SendMessageResult{RequestID: reqID, MessageID: msgID}, nil
}

func (a *chatBridgeAdapter) WaitForGeneration(conversationID int64, requestID string) error {
	return a.waitFn(conversationID, requestID)
}

func (a *chatBridgeAdapter) GetLatestReply(conversationID int64) (string, error) {
	return a.getLatestReplyFn(conversationID)
}

// NewChatBridge creates a ChatBridge from function callbacks.
// Used by bootstrap to wire services without cyclic imports.
func NewChatBridge(
	findOrCreateConvFn func(agentID int64, externalID, name string) (int64, error),
	sendFn func(convID int64, content, tabID string) (requestID string, messageID int64, err error),
	waitFn func(convID int64, requestID string) error,
	getLatestReplyFn func(convID int64) (string, error),
) ChatBridge {
	return &chatBridgeAdapter{
		findOrCreateConvFn: findOrCreateConvFn,
		sendFn:             sendFn,
		waitFn:             waitFn,
		getLatestReplyFn:   getLatestReplyFn,
	}
}
