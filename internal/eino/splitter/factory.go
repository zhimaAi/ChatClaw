package splitter

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/semantic"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/embedding"
)

// Config 创建分割器的配置
type Config struct {
	// FilePath 文件路径，用于根据扩展名选择合适的分割器
	FilePath string
	// ChunkSize 每个分块的目标大小（按字符数计算）
	ChunkSize int
	// ChunkOverlap 相邻分块之间的重叠大小（按字符数计算）
	ChunkOverlap int
	// SemanticEmbedder 语义分割使用的嵌入模型（可选）
	// 如果提供，将使用语义分割而非递归分割
	SemanticEmbedder embedding.Embedder
	// SemanticPercentile 语义分割的百分位阈值（0-1）
	// 值越高，分割点越少。默认为 0.9
	SemanticPercentile float64
	// SemanticMinChunkSize 语义分割的最小分块大小
	// 默认为 100
	SemanticMinChunkSize int
}

// DefaultSeparators 递归分割的默认分隔符
var DefaultSeparators = []string{
	"\n\n", // 段落分隔
	"\n",   // 换行符
	"。",   // 中文句号
	"！",   // 中文感叹号
	"？",   // 中文问号
	"；",   // 中文分号
	".",    // 英文句号
	"!",    // 英文感叹号
	"?",    // 英文问号
	";",    // 英文分号
	" ",    // 空格
	"",     // 逐字符分割
}

// NewSplitter 根据配置创建新的文档分割器
// 优先级：Markdown Header Splitter > Semantic Splitter > Recursive Splitter
func NewSplitter(ctx context.Context, cfg *Config) (document.Transformer, error) {
	if cfg == nil {
		cfg = &Config{
			ChunkSize:    512,
			ChunkOverlap: 50,
		}
	}

	// 使用字符长度计算（适用于中文）
	lenFunc := func(s string) int {
		return len([]rune(s))
	}

	// 检查是否为 Markdown 文件，使用专门的 Header Splitter
	if cfg.FilePath != "" {
		ext := strings.ToLower(filepath.Ext(cfg.FilePath))
		if ext == ".md" || ext == ".markdown" {
			return NewMarkdownSplitter(ctx)
		}
	}

	// 如果提供了语义嵌入模型，使用语义分割
	if cfg.SemanticEmbedder != nil {
		percentile := cfg.SemanticPercentile
		if percentile <= 0 || percentile > 1 {
			percentile = 0.9
		}
		minChunkSize := cfg.SemanticMinChunkSize
		if minChunkSize <= 0 {
			minChunkSize = 100
		}

		return semantic.NewSplitter(ctx, &semantic.Config{
			Embedding:    cfg.SemanticEmbedder,
			BufferSize:   2,
			MinChunkSize: minChunkSize,
			Separators:   DefaultSeparators,
			Percentile:   percentile,
			LenFunc:      lenFunc,
		})
	}

	// 默认使用递归分割
	// Apply defaults if values are not set
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 512
	}
	if cfg.ChunkOverlap < 0 {
		cfg.ChunkOverlap = 50
	}

	return recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize:   cfg.ChunkSize,
		OverlapSize: cfg.ChunkOverlap,
		Separators:  DefaultSeparators,
		LenFunc:     lenFunc,
		KeepType:    recursive.KeepTypeEnd,
	})
}

// NewRecursiveSplitter 使用给定配置创建递归分割器
func NewRecursiveSplitter(ctx context.Context, chunkSize, chunkOverlap int) (document.Transformer, error) {
	return NewSplitter(ctx, &Config{
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
	})
}

// NewSemanticSplitter 使用给定的嵌入模型创建语义分割器
func NewSemanticSplitter(ctx context.Context, embedder embedding.Embedder, percentile float64) (document.Transformer, error) {
	return NewSplitter(ctx, &Config{
		SemanticEmbedder:   embedder,
		SemanticPercentile: percentile,
	})
}

// NewMarkdownSplitter 创建 Markdown Header 分割器
// 按标题层级（#, ##, ###）进行结构化分割
func NewMarkdownSplitter(ctx context.Context) (document.Transformer, error) {
	return markdown.NewHeaderSplitter(ctx, &markdown.HeaderConfig{
		Headers: map[string]string{
			"#":    "h1",
			"##":   "h2",
			"###":  "h3",
			"####": "h4",
		},
		TrimHeaders: false, // 保留标题行在内容中
	})
}
