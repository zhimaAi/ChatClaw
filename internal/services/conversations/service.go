package conversations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"willchat/internal/errs"
	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ConversationsService 会话服务（暴露给前端调用）
type ConversationsService struct {
	app *application.App
}

func NewConversationsService(app *application.App) *ConversationsService {
	return &ConversationsService{app: app}
}

func (s *ConversationsService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// serializeLibraryIDs converts library IDs to JSON string for database storage
func serializeLibraryIDs(ids []int64) string {
	if len(ids) == 0 {
		return "[]"
	}
	jsonBytes, err := json.Marshal(ids)
	if err != nil {
		// This should rarely happen with []int64, but log it for debugging
		log.Printf("[conversations] failed to serialize library_ids: %v", err)
		return "[]"
	}
	return string(jsonBytes)
}

// ListConversations 获取指定助手的会话列表（置顶优先，然后按更新时间倒序）
func (s *ConversationsService) ListConversations(agentID int64) ([]Conversation, error) {
	if agentID <= 0 {
		return nil, errs.New("error.agent_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models := make([]conversationModel, 0)
	if err := db.NewSelect().
		Model(&models).
		Where("agent_id = ?", agentID).
		OrderExpr("is_pinned DESC, updated_at DESC, id DESC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.conversation_list_failed", err)
	}

	out := make([]Conversation, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

// GetConversation 获取单个会话
func (s *ConversationsService) GetConversation(id int64) (*Conversation, error) {
	if id <= 0 {
		return nil, errs.New("error.conversation_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m conversationModel
	if err := db.NewSelect().
		Model(&m).
		Where("id = ?", id).
		Limit(1).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.conversation_not_found", map[string]any{"ID": id})
		}
		return nil, errs.Wrap("error.conversation_read_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// CreateConversation 创建会话
func (s *ConversationsService) CreateConversation(input CreateConversationInput) (*Conversation, error) {
	if input.AgentID <= 0 {
		return nil, errs.New("error.agent_id_required")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.conversation_name_required")
	}
	// 截取前 100 个字符作为会话名称
	nameRunes := []rune(name)
	if len(nameRunes) > 100 {
		name = string(nameRunes[:100])
	}

	lastMessage := strings.TrimSpace(input.LastMessage)

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 验证助手是否存在
	var agentCount int
	if err := db.NewSelect().
		Table("agents").
		ColumnExpr("COUNT(1)").
		Where("id = ?", input.AgentID).
		Scan(ctx, &agentCount); err != nil {
		return nil, errs.Wrap("error.conversation_create_failed", err)
	}
	if agentCount == 0 {
		return nil, errs.Newf("error.agent_not_found", map[string]any{"ID": input.AgentID})
	}

	m := &conversationModel{
		AgentID:       input.AgentID,
		Name:          name,
		LastMessage:   lastMessage,
		IsPinned:      false,
		LLMProviderID: strings.TrimSpace(input.LLMProviderID),
		LLMModelID:    strings.TrimSpace(input.LLMModelID),
		LibraryIDs:    serializeLibraryIDs(input.LibraryIDs),
	}

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.conversation_create_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// UpdateConversation 更新会话（重命名、更新最后一条消息、置顶状态）
// 注意：每个助手只能有一个置顶会话，置顶新会话时会自动取消该助手下其他会话的置顶
func (s *ConversationsService) UpdateConversation(id int64, input UpdateConversationInput) (*Conversation, error) {
	if id <= 0 {
		return nil, errs.New("error.conversation_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Use transaction to ensure consistency when pinning
	var result *Conversation
	txErr := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// 如果要置顶，先获取该会话的 agent_id，然后取消该 agent 下所有其他会话的置顶
		if input.IsPinned != nil && *input.IsPinned {
			var agentID int64
			if err := tx.NewSelect().
				Table("conversations").
				Column("agent_id").
				Where("id = ?", id).
				Limit(1).
				Scan(ctx, &agentID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return errs.Newf("error.conversation_not_found", map[string]any{"ID": id})
				}
				return errs.Wrap("error.conversation_read_failed", err)
			}

			// 取消该 agent 下所有其他会话的置顶
			if _, err := tx.NewUpdate().
				Model((*conversationModel)(nil)).
				Where("agent_id = ?", agentID).
				Where("id != ?", id).
				Where("is_pinned = ?", true).
				Set("is_pinned = ?", false).
				Exec(ctx); err != nil {
				return errs.Wrap("error.conversation_update_failed", err)
			}
		}

		q := tx.NewUpdate().
			Model((*conversationModel)(nil)).
			Where("id = ?", id)

		if input.Name != nil {
			name := strings.TrimSpace(*input.Name)
			if name == "" {
				return errs.New("error.conversation_name_required")
			}
			// 截取前 100 个字符
			nameRunes := []rune(name)
			if len(nameRunes) > 100 {
				name = string(nameRunes[:100])
			}
			q = q.Set("name = ?", name)
		}

		if input.LastMessage != nil {
			q = q.Set("last_message = ?", strings.TrimSpace(*input.LastMessage))
		}

		if input.IsPinned != nil {
			q = q.Set("is_pinned = ?", *input.IsPinned)
		}

		if input.LLMProviderID != nil {
			q = q.Set("llm_provider_id = ?", strings.TrimSpace(*input.LLMProviderID))
		}

		if input.LLMModelID != nil {
			q = q.Set("llm_model_id = ?", strings.TrimSpace(*input.LLMModelID))
		}

		if input.LibraryIDs != nil {
			q = q.Set("library_ids = ?", serializeLibraryIDs(*input.LibraryIDs))
		}

		res, err := q.Exec(ctx)
		if err != nil {
			return errs.Wrap("error.conversation_update_failed", err)
		}

		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			return errs.Newf("error.conversation_not_found", map[string]any{"ID": id})
		}

		return nil
	})

	if txErr != nil {
		return nil, txErr
	}

	result, err = s.GetConversation(id)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteConversation 删除会话
func (s *ConversationsService) DeleteConversation(id int64) error {
	if id <= 0 {
		return errs.New("error.conversation_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := db.NewDelete().
		Model((*conversationModel)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.conversation_delete_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errs.Newf("error.conversation_not_found", map[string]any{"ID": id})
	}
	return nil
}

// DeleteConversationsByAgentID 删除指定助手的所有会话（用于删除助手时清理）
func (s *ConversationsService) DeleteConversationsByAgentID(agentID int64) error {
	if agentID <= 0 {
		return errs.New("error.agent_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 删除所有该助手的会话
	_, err = db.NewDelete().
		Model((*conversationModel)(nil)).
		Where("agent_id = ?", agentID).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.conversation_delete_failed", err)
	}

	return nil
}
