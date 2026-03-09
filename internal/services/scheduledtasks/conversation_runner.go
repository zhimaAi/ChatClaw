package scheduledtasks

import (
	"context"
	"fmt"
	"time"

	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
)

type conversationService interface {
	CreateConversation(input conversations.CreateConversationInput) (*conversations.Conversation, error)
	GetConversation(id int64) (*conversations.Conversation, error)
}

type chatService interface {
	SendMessage(input chat.SendMessageInput) (*chat.SendMessageResult, error)
	GetMessages(conversationID int64) ([]chat.Message, error)
}

type runDependencies struct {
	conversations conversationService
	chat          chatService
}

func (s *ScheduledTasksService) executeTask(ctx context.Context, task scheduledTaskModel, triggerType string) (*scheduledTaskRunModel, error) {
	runModel, err := s.createRun(ctx, task, triggerType)
	if err != nil {
		return nil, err
	}

	startedAt := time.Now()
	conversationName := fmt.Sprintf("(定时) %s - %s", task.Name, startedAt.Format("2006-01-02 15:04"))
	conv, err := s.runnerDeps.conversations.CreateConversation(conversations.CreateConversationInput{
		AgentID:        task.AgentID,
		Name:           conversationName,
		LLMProviderID:  task.LLMProviderID,
		LLMModelID:     task.LLMModelID,
		LibraryIDs:     parseInt64Array(task.LibraryIDs),
		EnableThinking: task.EnableThinking,
		ChatMode:       task.ChatMode,
	})
	if err != nil {
		_ = s.failRun(context.Background(), task.ID, runModel.ID, err.Error(), startedAt)
		return nil, err
	}

	runModel.ConversationID = &conv.ID
	if err := s.updateRunConversation(ctx, runModel.ID, conv.ID); err != nil {
		return nil, err
	}

	sendResult, err := s.runnerDeps.chat.SendMessage(chat.SendMessageInput{
		ConversationID: conv.ID,
		Content:        task.Prompt,
		TabID:          fmt.Sprintf("scheduled-task-%d", task.ID),
	})
	if err != nil {
		_ = s.failRun(context.Background(), task.ID, runModel.ID, err.Error(), startedAt)
		return nil, err
	}

	runModel.UserMessageID = &sendResult.MessageID
	if err := s.markRunStarted(ctx, task.ID, runModel.ID, conv.ID, sendResult.MessageID); err != nil {
		return nil, err
	}

	go s.watchRun(task.ID, runModel.ID, conv.ID, startedAt)

	return runModel, nil
}
