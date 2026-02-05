package tools

import (
	"context"
	"time"

	"github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/tool"
)

// DuckDuckGoConfig defines the configuration for the DuckDuckGo search tool.
type DuckDuckGoConfig struct {
	MaxResults int
	Timeout    time.Duration
}

// DefaultDuckDuckGoConfig returns the default configuration.
func DefaultDuckDuckGoConfig() *DuckDuckGoConfig {
	return &DuckDuckGoConfig{
		MaxResults: 5,
		Timeout:    10 * time.Second,
	}
}

// NewDuckDuckGoTool creates a new DuckDuckGo search tool.
func NewDuckDuckGoTool(ctx context.Context, config *DuckDuckGoConfig) (tool.InvokableTool, error) {
	if config == nil {
		config = DefaultDuckDuckGoConfig()
	}

	return duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		ToolName:   ToolIDDuckDuckGoSearch,
		ToolDesc:   "Search the web for information using DuckDuckGo. Use this tool when you need to find current information, facts, or data from the internet.",
		Region:     duckduckgo.RegionWT, // Worldwide
		MaxResults: config.MaxResults,
		Timeout:    config.Timeout,
	})
}
