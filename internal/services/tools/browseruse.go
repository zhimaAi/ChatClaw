package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino-ext/components/tool/browseruse"
	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	// browserUseCallTimeout is the max duration for a single browser tool invocation.
	browserUseCallTimeout = 60 * time.Second
)

// browserUseWrapper wraps BrowserUseTool with context cancellation and timeout support.
//
// It addresses two issues with the underlying browseruse.Tool:
//  1. No context cancellation — browser operations ignore the call-level context.
//  2. No timeout — operations can hang indefinitely.
type browserUseWrapper struct {
	inner *browseruse.Tool
}

func (w *browserUseWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.inner.Info(ctx)
}

func (w *browserUseWrapper) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	type callResult struct {
		output string
		err    error
	}

	ch := make(chan callResult, 1)
	go func() {
		out, err := w.inner.InvokableRun(ctx, argumentsInJSON, opts...)
		ch <- callResult{out, err}
	}()

	// Apply per-call timeout on top of the caller's context.
	timeoutCtx, cancel := context.WithTimeout(ctx, browserUseCallTimeout)
	defer cancel()

	select {
	case <-timeoutCtx.Done():
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("browser operation timed out after %v", browserUseCallTimeout)
	case r := <-ch:
		return r.output, r.err
	}
}

// BrowserUseConfig defines configuration for the browser use tool.
type BrowserUseConfig struct {
	// ExtractChatModel is used to intelligently extract content from web pages.
	// If not set, extract_content action will return raw HTML which can be very large.
	ExtractChatModel model.BaseChatModel
}

// NewBrowserUseTool creates a new browser use tool for web browsing automation.
// It enables the agent to navigate websites, interact with web pages, and perform web searches.
// The tool runs Chrome in headless mode and is wrapped with cancellation/timeout support.
func NewBrowserUseTool(ctx context.Context, config *BrowserUseConfig) (tool.BaseTool, error) {
	// Create a DuckDuckGo search client for the browser's web_search action.
	ddgSearch, err := duckduckgo.NewSearch(ctx, &duckduckgo.Config{
		MaxResults: 5,
		Region:     duckduckgo.RegionWT,
	})
	if err != nil {
		return nil, err
	}

	browserConfig := &browseruse.Config{
		Headless:      true,
		DDGSearchTool: ddgSearch,
	}

	// Configure ExtractChatModel if provided
	if config != nil && config.ExtractChatModel != nil {
		browserConfig.ExtractChatModel = config.ExtractChatModel
	}

	inner, err := browseruse.NewBrowserUseTool(ctx, browserConfig)
	if err != nil {
		return nil, err
	}

	return &browserUseWrapper{inner: inner}, nil
}
