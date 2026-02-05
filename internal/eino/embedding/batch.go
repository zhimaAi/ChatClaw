package embedding

import (
	"context"
	"fmt"

	einoembedding "github.com/cloudwego/eino/components/embedding"
)

// batchEmbedder wraps an Embedder and enforces a maximum batch size.
// This is critical for providers that restrict input.contents length (e.g. Qwen <= 10).
type batchEmbedder struct {
	inner   einoembedding.Embedder
	maxSize int
}

func WrapWithBatchLimit(inner einoembedding.Embedder, maxSize int) einoembedding.Embedder {
	if inner == nil {
		return nil
	}
	if maxSize <= 0 {
		return inner
	}
	return &batchEmbedder{inner: inner, maxSize: maxSize}
}

func (b *batchEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...einoembedding.Option) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if len(texts) <= b.maxSize {
		return b.inner.EmbedStrings(ctx, texts, opts...)
	}

	out := make([][]float64, 0, len(texts))
	for i := 0; i < len(texts); i += b.maxSize {
		end := i + b.maxSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := b.inner.EmbedStrings(ctx, texts[i:end], opts...)
		if err != nil {
			return nil, fmt.Errorf("embed batch %d-%d: %w", i+1, end, err)
		}
		out = append(out, vecs...)
	}
	return out, nil
}

