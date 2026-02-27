package chat

import (
	"context"
	"fmt"

	einoembed "chatclaw/internal/eino/embedding"
	"chatclaw/internal/eino/processor"
	"chatclaw/internal/eino/tools"
	"chatclaw/internal/services/retrieval"

	"github.com/cloudwego/eino/components/tool"
	"github.com/uptrace/bun"
)

// createLibraryRetrieverTool creates a LibraryRetrieverTool for the given library IDs
func (s *ChatService) createLibraryRetrieverTool(ctx context.Context, db *bun.DB, libraryIDs []int64, topK int, matchThreshold float64) (tool.BaseTool, error) {
	if len(libraryIDs) == 0 {
		return nil, nil
	}

	embeddingConfig, err := processor.GetEmbeddingConfig(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("get embedding config: %w", err)
	}

	embedder, err := einoembed.NewEmbedder(ctx, &einoembed.ProviderConfig{
		ProviderType: embeddingConfig.ProviderType,
		APIKey:       embeddingConfig.APIKey,
		APIEndpoint:  embeddingConfig.APIEndpoint,
		ModelID:      embeddingConfig.ModelID,
		Dimension:    embeddingConfig.Dimension,
		ExtraConfig:  embeddingConfig.ExtraConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("create embedder: %w", err)
	}
	if embedder == nil {
		return nil, fmt.Errorf("embedder is nil after creation")
	}

	retrievalService := retrieval.NewService(db, embedder)

	if topK <= 0 {
		topK = 10
	}

	retrieverTool, err := tools.NewLibraryRetrieverTool(ctx, &tools.LibraryRetrieverConfig{
		LibraryIDs:     libraryIDs,
		TopK:           topK,
		MatchThreshold: matchThreshold,
		Retriever:      retrievalService,
	})
	if err != nil {
		return nil, fmt.Errorf("create library retriever tool: %w", err)
	}

	return retrieverTool, nil
}
