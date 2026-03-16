package chatwiki

import (
	"encoding/json"
	"testing"
)

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
