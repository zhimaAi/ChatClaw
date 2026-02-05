package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/tool/browseruse"
	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	// browserUseCallTimeout is the max duration for a single browser tool invocation.
	browserUseCallTimeout = 60 * time.Second

	// browserUseMaxResultChars limits tool result size to avoid overwhelming the LLM context.
	// The extract_content action can return 400KB+ of raw HTML when ExtractChatModel is not set.
	// 16000 chars ≈ 4000-8000 tokens, safely within most model context windows.
	browserUseMaxResultChars = 16000
)

// htmlTagRe matches HTML tags for stripping.
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// browserUseWrapper wraps BrowserUseTool with context cancellation, timeout, and
// result truncation support.
//
// It addresses three issues with the underlying browseruse.Tool:
//  1. No context cancellation — browser operations ignore the call-level context.
//  2. No timeout — operations can hang indefinitely.
//  3. Huge results — extract_content returns raw HTML (400KB+) when ExtractChatModel is nil,
//     which can exceed the LLM context window and cause long processing delays.
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
		if r.err != nil {
			return r.output, r.err
		}
		return truncateResult(r.output), nil
	}
}

// truncateResult limits the tool result size and strips HTML tags if the result
// is too large. This prevents the extract_content action from sending 400KB+ of
// raw HTML to the LLM.
func truncateResult(result string) string {
	if len(result) <= browserUseMaxResultChars {
		return result
	}

	// The result is too large (likely raw HTML from extract_content).
	// Strip HTML tags first to extract useful text content.
	text := htmlTagRe.ReplaceAllString(result, " ")

	// Collapse multiple whitespace into single space.
	text = collapseWhitespace(text)

	// If still too long after stripping, truncate.
	if len(text) > browserUseMaxResultChars {
		text = text[:browserUseMaxResultChars] + "\n\n[Content truncated due to size limit. The original page content was too large to include in full.]"
	}

	return text
}

// collapseWhitespace replaces runs of whitespace with a single space and trims.
func collapseWhitespace(s string) string {
	// Replace newlines, tabs, and multiple spaces with a single space.
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}

// NewBrowserUseTool creates a new browser use tool for web browsing automation.
// It enables the agent to navigate websites, interact with web pages, and perform web searches.
// The tool runs Chrome in headless mode and is wrapped with cancellation/timeout/truncation support.
func NewBrowserUseTool(ctx context.Context) (tool.BaseTool, error) {
	// Create a DuckDuckGo search client for the browser's web_search action.
	ddgSearch, err := duckduckgo.NewSearch(ctx, &duckduckgo.Config{
		MaxResults: 5,
		Region:     duckduckgo.RegionWT,
	})
	if err != nil {
		return nil, err
	}

	inner, err := browseruse.NewBrowserUseTool(ctx, &browseruse.Config{
		Headless:      true,
		DDGSearchTool: ddgSearch,
	})
	if err != nil {
		return nil, err
	}

	return &browserUseWrapper{inner: inner}, nil
}
