package agents

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func TestListAgentsForMatching(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "销售助手")
	seedAgent(t, db, 2, "日报助手")

	svc := NewAgentsServiceForTest(db)

	agents, err := svc.ListAgentsForMatching()
	if err != nil {
		t.Fatalf("ListAgentsForMatching returned error: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	if agents[0].ID == 0 || agents[0].Name == "" {
		t.Fatalf("expected minimal agent fields, got %+v", agents[0])
	}
}

func TestMatchAgentsByName(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "销售助手")
	seedAgent(t, db, 2, "销售日报助手")
	seedAgent(t, db, 3, "日报助手")

	svc := NewAgentsServiceForTest(db)

	tests := []struct {
		name         string
		query        string
		wantStatus   string
		wantCount    int
		wantFirstID  int64
	}{
		{name: "exact", query: "销售助手", wantStatus: "exact", wantCount: 1, wantFirstID: 1},
		{name: "multiple", query: "日报", wantStatus: "multiple", wantCount: 2},
		{name: "none", query: "不存在", wantStatus: "none", wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, status, err := svc.MatchAgentsByName(tt.query)
			if err != nil {
				t.Fatalf("MatchAgentsByName returned error: %v", err)
			}
			if status != tt.wantStatus {
				t.Fatalf("unexpected status: %s", status)
			}
			if len(matches) != tt.wantCount {
				t.Fatalf("unexpected match count: %d", len(matches))
			}
			if tt.wantFirstID > 0 && matches[0].ID != tt.wantFirstID {
				t.Fatalf("unexpected first match: %+v", matches[0])
			}
		})
	}
}

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	db := bun.NewDB(sqlDB, sqlitedialect.New())

	schema := `
create table agents (
	id integer primary key,
	name text not null default '',
	prompt text not null default '',
	icon text not null default '',
	default_llm_provider_id text not null default '',
	default_llm_model_id text not null default '',
	llm_temperature real not null default 0,
	llm_top_p real not null default 0,
	llm_max_context_count integer not null default 0,
	llm_max_tokens integer not null default 0,
	enable_llm_temperature boolean not null default false,
	enable_llm_top_p boolean not null default false,
	enable_llm_max_tokens boolean not null default false,
	retrieval_match_threshold real not null default 0,
	retrieval_top_k integer not null default 0,
	sandbox_mode text not null default '',
	sandbox_network boolean not null default false,
	work_dir text not null default '',
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp
);`
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("schema exec failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func seedAgent(t *testing.T, db *bun.DB, id int64, name string) {
	t.Helper()
	query := `insert into agents(id, name, prompt) values(?, ?, ?)`
	if _, err := db.ExecContext(context.Background(), query, id, name, "prompt"); err != nil {
		t.Fatalf("seed agent failed: %v", err)
	}
}
