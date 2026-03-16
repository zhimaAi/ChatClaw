package providers

import (
	"testing"

	"chatclaw/internal/services/chatwiki"
)

func TestBuildChatWikiProviderWithModels_OnlyReturnsLLMAndEmbedding(t *testing.T) {
	svc := &ProvidersService{}
	provider := &Provider{
		ProviderID: "chatwiki",
		Name:       "Chatwiki",
		Type:       "openai",
	}
	catalog := &chatwiki.ModelCatalog{
		LLMModels: []chatwiki.ModelCatalogItem{
			{ModelID: "chatwiki-llm", Name: "chatwiki-llm", Type: "llm", Enabled: true},
		},
		EmbeddingModels: []chatwiki.ModelCatalogItem{
			{ModelID: "chatwiki-embedding", Name: "chatwiki-embedding", Type: "embedding", Enabled: true},
		},
		RerankModels: []chatwiki.ModelCatalogItem{
			{ModelID: "chatwiki-rerank", Name: "chatwiki-rerank", Type: "rerank", Enabled: true},
		},
	}

	result := svc.buildChatWikiProviderWithModels(provider, catalog)
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if len(result.ModelGroups) != 2 {
		t.Fatalf("expected 2 model groups, got %#v", result.ModelGroups)
	}
	if result.ModelGroups[0].Type != "llm" || len(result.ModelGroups[0].Models) != 1 {
		t.Fatalf("expected llm group first, got %#v", result.ModelGroups)
	}
	if result.ModelGroups[1].Type != "embedding" || len(result.ModelGroups[1].Models) != 1 {
		t.Fatalf("expected embedding group second, got %#v", result.ModelGroups)
	}
}

func TestBuildChatWikiProviderWithModels_PreservesSupplierAndUnifiedName(t *testing.T) {
	svc := &ProvidersService{}
	provider := &Provider{
		ProviderID: "chatwiki",
		Name:       "Chatwiki",
		Type:       "openai",
	}
	catalog := &chatwiki.ModelCatalog{
		LLMModels: []chatwiki.ModelCatalogItem{
			{
				ModelID:       "deepseek-r1",
				Name:          "12",
				ModelSupplier: "deepseek-ai",
				UniModelName:  "DeepSeek-R1",
				Type:          "llm",
				Enabled:       true,
			},
		},
	}

	result := svc.buildChatWikiProviderWithModels(provider, catalog)
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if len(result.ModelGroups) != 1 || len(result.ModelGroups[0].Models) != 1 {
		t.Fatalf("expected one llm model, got %#v", result.ModelGroups)
	}

	model := result.ModelGroups[0].Models[0]
	if model.Name != "12" {
		t.Fatalf("expected raw name to be preserved, got %#v", model)
	}
	if model.ModelSupplier != "deepseek-ai" || model.UniModelName != "DeepSeek-R1" {
		t.Fatalf("expected supplier and unified name to be preserved, got %#v", model)
	}
}
