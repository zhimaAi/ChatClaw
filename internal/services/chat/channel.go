package chat

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	einoagent "chatclaw/internal/eino/agent"

	"github.com/cloudwego/eino/schema"
)

// SendChannelMessage performs a synchronous chat-mode LLM generation for a
// channel message. It inserts the user message, loads context, calls the model,
// persists the assistant reply, and returns the response text.
//
// Unlike SendMessage (which is async and emits frontend events), this method
// blocks until the AI response is fully generated and does not emit any events.
func (s *ChatService) SendChannelMessage(ctx context.Context, conversationID int64, content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("empty content")
	}

	db, err := s.db()
	if err != nil {
		return "", err
	}

	agentConfig, providerConfig, _, err := s.getAgentAndProviderConfig(ctx, db, conversationID)
	if err != nil {
		return "", fmt.Errorf("get config: %w", err)
	}

	// Insert user message
	userMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleUser,
		Content:        content,
		Status:         StatusSuccess,
		ToolCalls:      "[]",
	}
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if _, err := db.NewInsert().Model(userMsg).Exec(dbCtx); err != nil {
		dbCancel()
		return "", fmt.Errorf("save user message: %w", err)
	}
	dbCancel()

	// Load conversation history
	messages, err := s.loadMessagesForContext(
		ctx,
		db,
		conversationID,
		agentConfig.ContextCount,
		providerConfig.ProviderID,
		agentConfig.ModelID,
	)
	if err != nil {
		return "", fmt.Errorf("load history: %w", err)
	}

	messages = patchToolCallsForChatMode(messages)

	instruction := agentConfig.Instruction
	agentConfig.Provider = providerConfig

	chatModel, err := einoagent.CreateChatModel(ctx, agentConfig)
	if err != nil {
		return "", fmt.Errorf("create chat model: %w", err)
	}

	fullMessages := make([]*schema.Message, 0, len(messages)+1)
	fullMessages = append(fullMessages, &schema.Message{
		Role:    schema.System,
		Content: instruction,
	})
	fullMessages = append(fullMessages, messages...)

	// Use streaming to collect the full response (Generate may not be
	// implemented by all providers in eino; Stream is universally supported).
	stream, err := chatModel.Stream(ctx, fullMessages)
	if err != nil {
		return "", fmt.Errorf("start stream: %w", err)
	}

	var responseBuilder strings.Builder
	var inputTokens, outputTokens int
	var finishReason string

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return "", fmt.Errorf("stream recv: %w", err)
		}
		if ctx.Err() != nil {
			break
		}

		if msg.Content != "" {
			responseBuilder.WriteString(msg.Content)
		}
		if msg.ResponseMeta != nil {
			if msg.ResponseMeta.FinishReason != "" {
				finishReason = msg.ResponseMeta.FinishReason
			}
			if msg.ResponseMeta.Usage != nil {
				inputTokens += int(msg.ResponseMeta.Usage.PromptTokens)
				outputTokens += int(msg.ResponseMeta.Usage.CompletionTokens)
			}
		}
	}

	responseContent := responseBuilder.String()

	// Persist assistant message
	status := StatusSuccess
	if ctx.Err() != nil {
		status = StatusCancelled
	}

	assistantMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleAssistant,
		Content:        responseContent,
		ProviderID:     providerConfig.ProviderID,
		ModelID:        agentConfig.ModelID,
		Status:         status,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
		FinishReason:   finishReason,
		ToolCalls:      "[]",
	}
	dbCtx2, dbCancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	if _, err := db.NewInsert().Model(assistantMsg).Exec(dbCtx2); err != nil {
		dbCancel2()
		s.app.Logger.Error("[chat] channel: save assistant message failed", "conv", conversationID, "error", err)
	}
	dbCancel2()

	s.app.Logger.Info("[chat] channel reply generated",
		"conv", conversationID,
		"response_len", len(responseContent),
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
	)

	return responseContent, nil
}
