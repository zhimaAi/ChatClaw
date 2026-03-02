package memory

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/fts/tokenizer"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type CoreProfile struct {
	bun.BaseModel `bun:"table:core_profiles"`

	ID        int64     `bun:"id,pk,autoincrement"`
	AgentID   int64     `bun:"agent_id"`
	Content   string    `bun:"content"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

var _ bun.BeforeInsertHook = (*CoreProfile)(nil)

func (*CoreProfile) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

type ThematicFact struct {
	bun.BaseModel `bun:"table:thematic_facts"`

	ID        int64     `bun:"id,pk,autoincrement"`
	AgentID   int64     `bun:"agent_id"`
	Topic     string    `bun:"topic"`
	Content   string    `bun:"content"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

var _ bun.BeforeInsertHook = (*ThematicFact)(nil)

func (*ThematicFact) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

type EventStream struct {
	bun.BaseModel `bun:"table:event_streams"`

	ID        int64     `bun:"id,pk,autoincrement"`
	AgentID   int64     `bun:"agent_id"`
	Date      string    `bun:"date"`
	Content   string    `bun:"content"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

var _ bun.BeforeInsertHook = (*EventStream)(nil)

func (*EventStream) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

// MemoryService 记忆服务（暴露给前端调用）
type MemoryService struct {
	app *application.App
}

func NewMemoryService(app *application.App) *MemoryService {
	return &MemoryService{app: app}
}

func (s *MemoryService) GetCoreProfile(ctx context.Context, agentID int64) (string, error) {
	return GetCoreProfileContent(ctx, agentID)
}

// GetCoreProfileContent returns the core profile text for an agent from memory DB.
// Used by chat service to inject Core Profile via ADK middleware.
func GetCoreProfileContent(ctx context.Context, agentID int64) (string, error) {
	if db == nil {
		return "", errs.New("error.memory_db_not_initialized")
	}
	var m CoreProfile
	err := db.NewSelect().
		Model(&m).
		Where("agent_id = ?", agentID).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return m.Content, nil
}

// ThematicFactDTO is the frontend-facing DTO for thematic facts.
type ThematicFactDTO struct {
	ID      int64  `json:"id"`
	Topic   string `json:"topic"`
	Content string `json:"content"`
}

// EventStreamDTO is the frontend-facing DTO for event stream entries.
type EventStreamDTO struct {
	ID      int64  `json:"id"`
	Date    string `json:"date"`
	Content string `json:"content"`
}

// EventStreamPageInput holds cursor-pagination parameters for event streams.
type EventStreamPageInput struct {
	AgentID    int64  `json:"agent_id"`
	BeforeDate string `json:"before_date"`
	BeforeID   int64  `json:"before_id"`
	Limit      int    `json:"limit"`
}

func (s *MemoryService) GetThematicFacts(ctx context.Context, agentID int64) ([]ThematicFactDTO, error) {
	if db == nil {
		return nil, errs.New("error.memory_db_not_initialized")
	}

	var facts []ThematicFact
	err := db.NewSelect().Model(&facts).
		Where("agent_id = ?", agentID).
		OrderExpr("updated_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ThematicFactDTO, len(facts))
	for i, f := range facts {
		result[i] = ThematicFactDTO{ID: f.ID, Topic: f.Topic, Content: f.Content}
	}
	return result, nil
}

// GetEventStreamsPage returns a cursor-paginated page of event streams.
// Uses (date DESC, id DESC) ordering with a composite cursor (before_date, before_id).
func (s *MemoryService) GetEventStreamsPage(ctx context.Context, input EventStreamPageInput) ([]EventStreamDTO, error) {
	if db == nil {
		return nil, errs.New("error.memory_db_not_initialized")
	}
	if input.AgentID <= 0 {
		return nil, errs.New("error.agent_id_required")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var events []EventStream
	q := db.NewSelect().Model(&events).
		Where("agent_id = ?", input.AgentID)

	if input.BeforeDate != "" && input.BeforeID > 0 {
		q = q.Where("(date < ? OR (date = ? AND id < ?))", input.BeforeDate, input.BeforeDate, input.BeforeID)
	}

	err := q.OrderExpr("date DESC, id DESC").Limit(limit).Scan(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]EventStreamDTO, len(events))
	for i, e := range events {
		result[i] = EventStreamDTO{ID: e.ID, Date: e.Date, Content: e.Content}
	}
	return result, nil
}

func (s *MemoryService) UpdateCoreProfile(ctx context.Context, agentID int64, content string) error {
	if db == nil {
		return errs.New("error.memory_db_not_initialized")
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return errs.New("error.memory_content_empty")
	}

	now := time.Now().UTC()
	var cp CoreProfile
	err := db.NewSelect().Model(&cp).Where("agent_id = ?", agentID).Limit(1).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			cp = CoreProfile{AgentID: agentID, Content: content}
			_, err = db.NewInsert().Model(&cp).Exec(ctx)
			return err
		}
		return err
	}

	cp.Content = content
	cp.UpdatedAt = now
	_, err = db.NewUpdate().Model(&cp).WherePK().Exec(ctx)
	return err
}

func (s *MemoryService) UpdateThematicFact(ctx context.Context, id int64, topic, content string) error {
	if db == nil {
		return errs.New("error.memory_db_not_initialized")
	}

	topic = strings.TrimSpace(topic)
	content = strings.TrimSpace(content)
	if topic == "" || content == "" {
		return errs.New("error.memory_content_empty")
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var existing ThematicFact
		if err := tx.NewSelect().Model(&existing).Where("id = ?", id).Scan(ctx); err != nil {
			return err
		}

		oldTokens := tokenizer.TokenizeContent(existing.Topic + " " + existing.Content)

		existing.Topic = topic
		existing.Content = content
		existing.UpdatedAt = time.Now().UTC()
		if _, err := tx.NewUpdate().Model(&existing).WherePK().Exec(ctx); err != nil {
			return err
		}

		newTokens := tokenizer.TokenizeContent(topic + " " + content)
		replaceTFFTS(ctx, tx, existing.ID, existing.AgentID, oldTokens, newTokens)
		deleteTFVec(ctx, tx, existing.ID)
		return nil
	})
}

func (s *MemoryService) DeleteThematicFact(ctx context.Context, id int64) error {
	if db == nil {
		return errs.New("error.memory_db_not_initialized")
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var existing ThematicFact
		if err := tx.NewSelect().Model(&existing).Where("id = ?", id).Scan(ctx); err != nil {
			return err
		}

		oldTokens := tokenizer.TokenizeContent(existing.Topic + " " + existing.Content)
		deleteTFFTS(ctx, tx, existing.ID, existing.AgentID, oldTokens)

		if _, err := tx.NewDelete().Model((*ThematicFact)(nil)).Where("id = ?", id).Exec(ctx); err != nil {
			return err
		}

		deleteTFVec(ctx, tx, id)
		return nil
	})
}

func (s *MemoryService) UpdateEventStream(ctx context.Context, id int64, content string) error {
	if db == nil {
		return errs.New("error.memory_db_not_initialized")
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return errs.New("error.memory_content_empty")
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var existing EventStream
		if err := tx.NewSelect().Model(&existing).Where("id = ?", id).Scan(ctx); err != nil {
			return err
		}

		oldTokens := tokenizer.TokenizeContent(existing.Content)

		existing.Content = content
		existing.UpdatedAt = time.Now().UTC()
		if _, err := tx.NewUpdate().Model(&existing).WherePK().Exec(ctx); err != nil {
			return err
		}

		newTokens := tokenizer.TokenizeContent(content)
		replaceESFTS(ctx, tx, existing.ID, existing.AgentID, oldTokens, newTokens)
		deleteESVec(ctx, tx, existing.ID)
		return nil
	})
}

func (s *MemoryService) DeleteEventStream(ctx context.Context, id int64) error {
	if db == nil {
		return errs.New("error.memory_db_not_initialized")
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var existing EventStream
		if err := tx.NewSelect().Model(&existing).Where("id = ?", id).Scan(ctx); err != nil {
			return err
		}

		oldTokens := tokenizer.TokenizeContent(existing.Content)
		deleteESFTS(ctx, tx, existing.ID, existing.AgentID, oldTokens)

		if _, err := tx.NewDelete().Model((*EventStream)(nil)).Where("id = ?", id).Exec(ctx); err != nil {
			return err
		}

		deleteESVec(ctx, tx, id)
		return nil
	})
}

// DeleteAgentMemories deletes all memories associated with an agent.
// Called by AgentsService when an agent is deleted.
func DeleteAgentMemories(ctx context.Context, agentID int64) error {
	if db == nil {
		return errs.New("error.memory_db_not_initialized")
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*CoreProfile)(nil)).Where("agent_id = ?", agentID).Exec(ctx); err != nil {
			return err
		}

		var tfs []ThematicFact
		if err := tx.NewSelect().Model(&tfs).Where("agent_id = ?", agentID).Scan(ctx); err != nil {
			return err
		}
		for _, tf := range tfs {
			tfTokens := tokenizer.TokenizeContent(tf.Topic + " " + tf.Content)
			deleteTFFTS(ctx, tx, tf.ID, tf.AgentID, tfTokens)
		}
		if len(tfs) > 0 {
			tfIDs := make([]int64, len(tfs))
			for i, tf := range tfs {
				tfIDs[i] = tf.ID
			}
			if _, err := tx.NewDelete().Model((*ThematicFact)(nil)).Where("agent_id = ?", agentID).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM thematic_facts_vec WHERE id IN (?)`, bun.In(tfIDs)); err != nil {
				return err
			}
		}

		var ess []EventStream
		if err := tx.NewSelect().Model(&ess).Where("agent_id = ?", agentID).Scan(ctx); err != nil {
			return err
		}
		for _, es := range ess {
			esTokens := tokenizer.TokenizeContent(es.Content)
			deleteESFTS(ctx, tx, es.ID, es.AgentID, esTokens)
		}
		if len(ess) > 0 {
			esIDs := make([]int64, len(ess))
			for i, es := range ess {
				esIDs[i] = es.ID
			}
			if _, err := tx.NewDelete().Model((*EventStream)(nil)).Where("agent_id = ?", agentID).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM event_streams_vec WHERE id IN (?)`, bun.In(esIDs)); err != nil {
				return err
			}
		}

		return nil
	})
}

// --- FTS / vector index helpers ---

func deleteTFFTS(ctx context.Context, tx bun.Tx, id, agentID int64, tokens string) {
	_, _ = tx.ExecContext(ctx,
		`INSERT INTO thematic_facts_fts(thematic_facts_fts, rowid, tokens, agent_id) VALUES('delete', ?, ?, ?)`,
		id, tokens, agentID)
}

func replaceTFFTS(ctx context.Context, tx bun.Tx, id, agentID int64, oldTokens, newTokens string) {
	deleteTFFTS(ctx, tx, id, agentID, oldTokens)
	_, _ = tx.ExecContext(ctx,
		`INSERT INTO thematic_facts_fts(rowid, tokens, agent_id) VALUES (?, ?, ?)`,
		id, newTokens, agentID)
}

func deleteTFVec(ctx context.Context, tx bun.Tx, id int64) {
	_, _ = tx.ExecContext(ctx, `DELETE FROM thematic_facts_vec WHERE id = ?`, id)
}

func deleteESFTS(ctx context.Context, tx bun.Tx, id, agentID int64, tokens string) {
	_, _ = tx.ExecContext(ctx,
		`INSERT INTO event_streams_fts(event_streams_fts, rowid, tokens, agent_id) VALUES('delete', ?, ?, ?)`,
		id, tokens, agentID)
}

func replaceESFTS(ctx context.Context, tx bun.Tx, id, agentID int64, oldTokens, newTokens string) {
	deleteESFTS(ctx, tx, id, agentID, oldTokens)
	_, _ = tx.ExecContext(ctx,
		`INSERT INTO event_streams_fts(rowid, tokens, agent_id) VALUES (?, ?, ?)`,
		id, newTokens, agentID)
}

func deleteESVec(ctx context.Context, tx bun.Tx, id int64) {
	_, _ = tx.ExecContext(ctx, `DELETE FROM event_streams_vec WHERE id = ?`, id)
}
