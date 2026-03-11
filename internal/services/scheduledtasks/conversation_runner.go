package scheduledtasks

import (
	"context"
	"fmt"

	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/services/i18n"
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
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}

	runModel, err := s.createRun(ctx, task, triggerType)
	if err != nil {
		return nil, err
	}

	conversationModel, err := s.resolveTaskConversationModel(ctx, db, task.AgentID)
	if err != nil {
		_ = s.failRun(context.Background(), task.ID, runModel.ID, err.Error(), runModel.StartedAt)
		return nil, err
	}

	startedAt := runModel.StartedAt
	conversationName := i18n.Tf("scheduled_task.conversation.name", map[string]any{
		"TaskName": task.Name,
		"StartedAt": startedAt.Format("2006-01-02 15:04"),
	})
	conv, err := s.runnerDeps.conversations.CreateConversation(conversations.CreateConversationInput{
		AgentID:        task.AgentID,
		Name:           conversationName,
		LLMProviderID:  conversationModel.ProviderID,
		LLMModelID:     conversationModel.ModelID,
		LibraryIDs:     []int64{},
		EnableThinking: false,
		ChatMode:       conversations.ChatModeTask,
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
