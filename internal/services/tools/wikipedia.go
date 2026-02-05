package tools

import (
	"context"
	"time"

	"github.com/cloudwego/eino-ext/components/tool/wikipedia"
	"github.com/cloudwego/eino/components/tool"
)

// WikipediaConfig defines the configuration for the Wikipedia search tool.
type WikipediaConfig struct {
	DocMaxChars int
	Timeout     time.Duration
	TopK        int
	Language    string
}

// DefaultWikipediaConfig returns the default configuration.
func DefaultWikipediaConfig() *WikipediaConfig {
	return &WikipediaConfig{
		DocMaxChars: 2000,
		Timeout:     15 * time.Second,
		TopK:        3,
		Language:    "en",
	}
}

// NewWikipediaTool creates a new Wikipedia search tool.
func NewWikipediaTool(ctx context.Context, config *WikipediaConfig) (tool.InvokableTool, error) {
	if config == nil {
		config = DefaultWikipediaConfig()
	}
	return wikipedia.NewTool(ctx, &wikipedia.Config{
		ToolName:    ToolIDWikipedia,
		ToolDesc:    "Search Wikipedia for information. Use this tool to look up facts, concepts, historical events, or any knowledge-based query.",
		DocMaxChars: config.DocMaxChars,
		Timeout:     config.Timeout,
		TopK:        config.TopK,
		Language:    config.Language,
	})
}
