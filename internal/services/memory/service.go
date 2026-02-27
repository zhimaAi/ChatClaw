package memory

import (
	"context"
	"database/sql"
	"errors"
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

func (s *MemoryService) GetEventStreams(ctx context.Context, agentID int64) ([]EventStreamDTO, error) {
	if db == nil {
		return nil, errs.New("error.memory_db_not_initialized")
	}

	var events []EventStream
	err := db.NewSelect().Model(&events).
		Where("agent_id = ?", agentID).
		OrderExpr("date DESC, id DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]EventStreamDTO, len(events))
	for i, e := range events {
		result[i] = EventStreamDTO{ID: e.ID, Date: e.Date, Content: e.Content}
	}
	return result, nil
}

// DeleteAgentMemories deletes all memories associated with an agent.
// Called by AgentsService when an agent is deleted.
func DeleteAgentMemories(ctx context.Context, agentID int64) error {
	if db == nil {
		return errs.New("error.memory_db_not_initialized")
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Delete core profiles
		if _, err := tx.NewDelete().Model((*CoreProfile)(nil)).Where("agent_id = ?", agentID).Exec(ctx); err != nil {
			return err
		}

		// Delete thematic facts and their FTS/vector entries
		var tfs []ThematicFact
		if err := tx.NewSelect().Model(&tfs).Where("agent_id = ?", agentID).Scan(ctx); err != nil {
			return err
		}
		for _, tf := range tfs {
			tfTokens := tokenizer.TokenizeContent(tf.Topic + " " + tf.Content)
			_, _ = tx.ExecContext(ctx,
				`INSERT INTO thematic_facts_fts(thematic_facts_fts, rowid, tokens, agent_id) VALUES('delete', ?, ?, ?)`,
				tf.ID, tfTokens, tf.AgentID)
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

		// Delete event streams and their FTS/vector entries
		var ess []EventStream
		if err := tx.NewSelect().Model(&ess).Where("agent_id = ?", agentID).Scan(ctx); err != nil {
			return err
		}
		for _, es := range ess {
			esTokens := tokenizer.TokenizeContent(es.Content)
			_, _ = tx.ExecContext(ctx,
				`INSERT INTO event_streams_fts(event_streams_fts, rowid, tokens, agent_id) VALUES('delete', ?, ?, ?)`,
				es.ID, esTokens, es.AgentID)
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
