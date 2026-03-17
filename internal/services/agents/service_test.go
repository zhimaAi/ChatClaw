package agents

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "github.com/mattn/go-sqlite3"
)

func TestEnsureLLMModelExists_AllowsChatWikiModelFromDB(t *testing.T) {
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite memory db: %v", err)
	}
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	defer db.Close()

	if _, err := db.Exec(`
create table models (
    id integer primary key autoincrement,
    created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp,
    provider_id varchar(64) not null,
    model_id varchar(128) not null,
    name varchar(128) not null,
    type varchar(16) not null default 'llm',
    capabilities text not null default '[]',
    is_builtin boolean not null default false,
    enabled boolean not null default true,
    sort_order integer not null default 0,
    unique(provider_id, model_id)
)`); err != nil {
		t.Fatalf("create models table: %v", err)
	}

	if _, err := db.Exec(`
insert into models (provider_id, model_id, name, type, enabled, is_builtin, sort_order, capabilities)
values ('chatwiki', 'deepseek-r1', 'deepseek-r1', 'llm', 1, 1, 0, '[]')`); err != nil {
		t.Fatalf("insert model: %v", err)
	}

	if err := ensureLLMModelExists(context.Background(), db, nil, "chatwiki", "deepseek-r1"); err != nil {
		t.Fatalf("ensureLLMModelExists returned error: %v", err)
	}
}
