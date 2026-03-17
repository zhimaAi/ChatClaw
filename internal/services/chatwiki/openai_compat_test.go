package chatwiki

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "github.com/mattn/go-sqlite3"
)

func TestResolveSelfOwnedModelConfigID_FetchesFromOpenAIEndpoint(t *testing.T) {
	openAIModelCatalogCache = sync.Map{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/manage/chatclaw/showModelConfigList" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Token"); got != "test-token" {
			t.Fatalf("unexpected token header: %q", got)
		}
		_, _ = w.Write([]byte(`{
			"data": {
				"language_models": [
					{"id": 12, "model_name": "deepseek-r1", "type": "llm", "enabled": 1}
				],
				"embedding_models": [
					{"id": 34, "model_name": "text-embedding", "type": "embedding", "enabled": 1}
				]
			}
		}`))
	}))
	defer server.Close()

	id, err := ResolveSelfOwnedModelConfigID("test-token", server.URL+"/chatclaw/v1", "deepseek-r1", "llm")
	if err != nil {
		t.Fatalf("ResolveSelfOwnedModelConfigID returned error: %v", err)
	}
	if id != 12 {
		t.Fatalf("expected config id 12, got %d", id)
	}

	embeddingID, err := ResolveSelfOwnedModelConfigID("test-token", server.URL+"/chatclaw/v1", "text-embedding", "embedding")
	if err != nil {
		t.Fatalf("ResolveSelfOwnedModelConfigID returned error for embedding: %v", err)
	}
	if embeddingID != 34 {
		t.Fatalf("expected embedding config id 34, got %d", embeddingID)
	}
}

func TestResolveSelfOwnedModelConfigID_ReturnsErrorWhenMissing(t *testing.T) {
	openAIModelCatalogCache = sync.Map{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"language_models":[]}}`))
	}))
	defer server.Close()

	_, err := ResolveSelfOwnedModelConfigID("test-token", server.URL+"/chatclaw/v1", "missing-model", "llm")
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestChatWikiChatCompletionsURL_UsesOpenAIPath(t *testing.T) {
	got := chatWikiChatCompletionsURL("https://example.com/base/")
	want := "https://example.com/base/chatclaw/v1/chat/completions"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLoadModelCatalogForOpenAI_SyncsModelsToDB(t *testing.T) {
	openAIModelCatalogCache = sync.Map{}

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite memory db: %v", err)
	}
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() {
		_ = db.Close()
	})
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
	prevGetDB := getChatWikiSyncDB
	getChatWikiSyncDB = func() *bun.DB { return db }
	t.Cleanup(func() {
		getChatWikiSyncDB = prevGetDB
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": {
				"language_models": [
					{"id": 12, "model_name": "deepseek-r1-raw", "uni_model_name": "deepseek-r1", "type": "llm", "enabled": 1}
				],
				"embedding_models": [
					{"id": 34, "model_name": "text-embedding-raw", "uni_model_name": "text-embedding-3-large", "type": "embedding", "enabled": 1}
				]
			}
		}`))
	}))
	defer server.Close()

	if _, err := loadModelCatalogForOpenAI("test-token", server.URL+"/chatclaw/v1"); err != nil {
		t.Fatalf("loadModelCatalogForOpenAI returned error: %v", err)
	}

	type storedModel struct {
		ModelID string `bun:"model_id"`
		Name    string `bun:"name"`
		Type    string `bun:"type"`
	}

	var models []storedModel
	if err := db.NewSelect().
		Table("models").
		Column("model_id", "name", "type").
		Where("provider_id = ?", "chatwiki").
		OrderExpr("type ASC, model_id ASC").
		Scan(context.Background(), &models); err != nil {
		t.Fatalf("select synced models: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 synced models, got %#v", models)
	}
	if models[0].ModelID != "text-embedding-3-large" || models[0].Name != "text-embedding-3-large" || models[0].Type != "embedding" {
		t.Fatalf("unexpected embedding row: %#v", models[0])
	}
	if models[1].ModelID != "deepseek-r1" || models[1].Name != "deepseek-r1" || models[1].Type != "llm" {
		t.Fatalf("unexpected llm row: %#v", models[1])
	}
}
