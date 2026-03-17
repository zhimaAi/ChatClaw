package chatwiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "github.com/mattn/go-sqlite3"
)

func TestGetModelCatalogSource_UsesDefaultServerURLWithoutBinding(t *testing.T) {
	source := getModelCatalogSource(nil)

	if source.Bound {
		t.Fatalf("expected unbound source, got %#v", source)
	}
	if source.ServerURL == "" {
		t.Fatalf("expected default server url, got %#v", source)
	}
	if source.ServerURL != "https://cloud.chatwiki.com" && source.ServerURL != "http://dev13.zhima_chat_ai.applnk.cn" {
		t.Fatalf("expected ChatWiki cloud url, got %#v", source)
	}
	if source.Token != "" {
		t.Fatalf("expected empty token for unbound source, got %#v", source)
	}
}

func TestGetModelCatalogSource_UsesBindingValuesWhenBound(t *testing.T) {
	source := getModelCatalogSource(&Binding{
		ServerURL: "https://example.com/openapi",
		Token:     "token-123",
		UserID:    "u1",
	})

	if !source.Bound {
		t.Fatalf("expected bound source, got %#v", source)
	}
	if source.ServerURL != "https://example.com/openapi" {
		t.Fatalf("unexpected source server url: %#v", source)
	}
	if source.Token != "token-123" {
		t.Fatalf("unexpected source token: %#v", source)
	}
}

func TestNormalizeManagementBaseURL_StripsOpenAPIPath(t *testing.T) {
	got := normalizeManagementBaseURL("https://chatclaw.chatwiki.com/openapi")
	if got != "https://chatclaw.chatwiki.com" {
		t.Fatalf("expected normalized management base url, got %q", got)
	}
}

func TestDecodeModelCatalogResponse_GroupsLLMAndEmbeddingOnly(t *testing.T) {
	raw := json.RawMessage(`{
		"res": 0,
		"data": {
			"language_models": [
				{"model_name": "chatwiki-llm", "type": "llm", "enabled": 1}
			],
			"embedding_models": [
				{"model_name": "chatwiki-embedding", "type": "embedding", "enabled": 1}
			],
			"rerank_models": [
				{"model_name": "chatwiki-rerank", "type": "rerank", "enabled": 1}
			]
		}
	}`)

	catalog, err := decodeModelCatalogResponse(raw)
	if err != nil {
		t.Fatalf("decodeModelCatalogResponse returned error: %v", err)
	}

	if len(catalog.LLMModels) != 1 || catalog.LLMModels[0].ModelID != "chatwiki-llm" {
		t.Fatalf("expected one llm model, got %#v", catalog.LLMModels)
	}
	if len(catalog.EmbeddingModels) != 1 || catalog.EmbeddingModels[0].ModelID != "chatwiki-embedding" {
		t.Fatalf("expected one embedding model, got %#v", catalog.EmbeddingModels)
	}
	if len(catalog.RerankModels) != 0 {
		t.Fatalf("expected rerank models to be ignored, got %#v", catalog.RerankModels)
	}
}

func TestDecodeModelCatalogResponse_ExtractsSupplierAndUnifiedName(t *testing.T) {
	raw := json.RawMessage(`{
		"data": {
			"language_models": [
				{
					"model_name": "qwen-plus",
					"display_name": "Qwen/Qwen-Plus",
					"type": "llm",
					"enabled": 1,
					"price": "2.00",
					"region_scope": "CN"
				},
				{
					"model_name": "deepseek-r1",
					"model_supplier": "deepseek-ai",
					"uni_model_name": "DeepSeek-R1",
					"type": "llm",
					"enabled": 1,
					"region_scope": "global"
				}
			]
		}
	}`)

	catalog, err := decodeModelCatalogResponse(raw)
	if err != nil {
		t.Fatalf("decodeModelCatalogResponse returned error: %v", err)
	}

	if len(catalog.LLMModels) != 2 {
		t.Fatalf("expected two llm models, got %#v", catalog.LLMModels)
	}

	if catalog.LLMModels[0].ModelSupplier != "Qwen" || catalog.LLMModels[0].UniModelName != "Qwen-Plus" {
		t.Fatalf("expected fallback split supplier/name, got %#v", catalog.LLMModels[0])
	}
	if catalog.LLMModels[0].RegionScope != "CN" {
		t.Fatalf("expected CN region scope, got %#v", catalog.LLMModels[0])
	}
	if catalog.LLMModels[0].Price != "2.00" {
		t.Fatalf("expected price to be kept as raw string, got %#v", catalog.LLMModels[0])
	}

	if catalog.LLMModels[1].ModelSupplier != "deepseek-ai" || catalog.LLMModels[1].UniModelName != "DeepSeek-R1" {
		t.Fatalf("expected explicit supplier/name, got %#v", catalog.LLMModels[1])
	}
	if catalog.LLMModels[1].RegionScope != "Global" {
		t.Fatalf("expected Global region scope, got %#v", catalog.LLMModels[1])
	}
}

func TestDecodeModelCatalogResponse_DetectsImageCapabilityFromInputImage(t *testing.T) {
	raw := json.RawMessage(`{
		"data": {
			"language_models": [
				{
					"model_name": "qwen-vl-plus",
					"type": "llm",
					"enabled": 1,
					"input_image": "1"
				}
			]
		}
	}`)

	catalog, err := decodeModelCatalogResponse(raw)
	if err != nil {
		t.Fatalf("decodeModelCatalogResponse returned error: %v", err)
	}

	if len(catalog.LLMModels) != 1 {
		t.Fatalf("expected one llm model, got %#v", catalog.LLMModels)
	}

	if len(catalog.LLMModels[0].Capabilities) != 2 {
		t.Fatalf("expected text and image capabilities, got %#v", catalog.LLMModels[0].Capabilities)
	}
	if catalog.LLMModels[0].Capabilities[0] != "text" || catalog.LLMModels[0].Capabilities[1] != "image" {
		t.Fatalf("expected image capability from input_image, got %#v", catalog.LLMModels[0].Capabilities)
	}
}

func TestDecodeModelCatalogResponse_DetectsFileCapabilityFromInputDocument(t *testing.T) {
	raw := json.RawMessage(`{
		"data": {
			"language_models": [
				{
					"model_name": "qwen-long",
					"type": "llm",
					"enabled": 1,
					"input_document": "1"
				}
			]
		}
	}`)

	catalog, err := decodeModelCatalogResponse(raw)
	if err != nil {
		t.Fatalf("decodeModelCatalogResponse returned error: %v", err)
	}

	if len(catalog.LLMModels) != 1 {
		t.Fatalf("expected one llm model, got %#v", catalog.LLMModels)
	}

	if len(catalog.LLMModels[0].Capabilities) != 2 {
		t.Fatalf("expected text and file capabilities, got %#v", catalog.LLMModels[0].Capabilities)
	}
	if catalog.LLMModels[0].Capabilities[0] != "text" || catalog.LLMModels[0].Capabilities[1] != "file" {
		t.Fatalf("expected file capability from input_document, got %#v", catalog.LLMModels[0].Capabilities)
	}
}

func TestDecodeModelCatalogResponse_ExtractsSelfOwnedModelConfigID(t *testing.T) {
	raw := json.RawMessage(`{
		"data": {
			"language_models": [
				{
					"id": 12,
					"model_name": "deepseek-r1",
					"type": "llm",
					"enabled": 1
				}
			]
		}
	}`)

	catalog, err := decodeModelCatalogResponse(raw)
	if err != nil {
		t.Fatalf("decodeModelCatalogResponse returned error: %v", err)
	}
	if len(catalog.LLMModels) != 1 {
		t.Fatalf("expected one llm model, got %#v", catalog.LLMModels)
	}
	if catalog.LLMModels[0].SelfOwnedModelConfigID != 12 {
		t.Fatalf("expected self owned model config id 12, got %#v", catalog.LLMModels[0])
	}
}

func TestSyncModelCatalogToDB_PersistsChatWikiModels(t *testing.T) {
	db := newChatWikiModelsTestDB(t)

	catalog := &ModelCatalog{
		LLMModels: []ModelCatalogItem{
			{ModelID: "ignored-llm-id", UniModelName: "deepseek-r1", Name: "DeepSeek R1", Type: "llm", Enabled: true},
		},
		EmbeddingModels: []ModelCatalogItem{
			{ModelID: "ignored-embedding-id", UniModelName: "text-embedding-3-large", Name: "Embedding", Type: "embedding", Enabled: true},
		},
	}

	if err := syncModelCatalogToDB(context.Background(), db, "chatwiki", catalog); err != nil {
		t.Fatalf("syncModelCatalogToDB returned error: %v", err)
	}

	type storedModel struct {
		ProviderID string `bun:"provider_id"`
		ModelID    string `bun:"model_id"`
		Name       string `bun:"name"`
		Type       string `bun:"type"`
	}

	var models []storedModel
	if err := db.NewSelect().
		Table("models").
		Column("provider_id", "model_id", "name", "type").
		OrderExpr("type ASC, model_id ASC").
		Scan(context.Background(), &models); err != nil {
		t.Fatalf("select synced models: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 synced models, got %#v", models)
	}
	if models[0].ProviderID != "chatwiki" || models[0].ModelID != "text-embedding-3-large" || models[0].Name != "text-embedding-3-large" || models[0].Type != "embedding" {
		t.Fatalf("unexpected embedding row: %#v", models[0])
	}
	if models[1].ProviderID != "chatwiki" || models[1].ModelID != "deepseek-r1" || models[1].Name != "deepseek-r1" || models[1].Type != "llm" {
		t.Fatalf("unexpected llm row: %#v", models[1])
	}
}

func TestSyncModelCatalogToDB_RemovesStaleChatWikiModels(t *testing.T) {
	db := newChatWikiModelsTestDB(t)

	initial := &ModelCatalog{
		LLMModels: []ModelCatalogItem{
			{UniModelName: "deepseek-r1", Type: "llm", Enabled: true},
			{UniModelName: "qwen-plus", Type: "llm", Enabled: true},
		},
	}
	if err := syncModelCatalogToDB(context.Background(), db, "chatwiki", initial); err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	next := &ModelCatalog{
		LLMModels: []ModelCatalogItem{
			{UniModelName: "deepseek-r1", Type: "llm", Enabled: true},
		},
	}
	if err := syncModelCatalogToDB(context.Background(), db, "chatwiki", next); err != nil {
		t.Fatalf("second sync failed: %v", err)
	}

	count, err := db.NewSelect().
		Table("models").
		Where("provider_id = ?", "chatwiki").
		Where("model_id = ?", "qwen-plus").
		Count(context.Background())
	if err != nil {
		t.Fatalf("count stale model: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected stale model to be removed, count=%d", count)
	}
}

func TestSyncModelCatalogToDB_PersistsFileCapabilityFromInputDocument(t *testing.T) {
	db := newChatWikiModelsTestDB(t)

	catalog := &ModelCatalog{
		LLMModels: []ModelCatalogItem{
			{
				ModelID:      "ignored-llm-id",
				UniModelName: "qwen-long",
				Name:         "Qwen Long",
				Type:         "llm",
				Enabled:      true,
				Capabilities: []string{"text", "file"},
			},
		},
	}

	if err := syncModelCatalogToDB(context.Background(), db, "chatwiki", catalog); err != nil {
		t.Fatalf("syncModelCatalogToDB returned error: %v", err)
	}

	type storedModel struct {
		ModelID      string `bun:"model_id"`
		Capabilities string `bun:"capabilities"`
	}

	var model storedModel
	if err := db.NewSelect().
		Table("models").
		Column("model_id", "capabilities").
		Where("provider_id = ?", "chatwiki").
		Where("model_id = ?", "qwen-long").
		Limit(1).
		Scan(context.Background(), &model); err != nil {
		t.Fatalf("select synced model: %v", err)
	}

	if model.ModelID != "qwen-long" {
		t.Fatalf("unexpected synced model id: %#v", model)
	}
	if model.Capabilities != `["text","file"]` {
		t.Fatalf("expected file capability to persist, got %#v", model)
	}
}

func newChatWikiModelsTestDB(t *testing.T) *bun.DB {
	t.Helper()

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

	return db
}
