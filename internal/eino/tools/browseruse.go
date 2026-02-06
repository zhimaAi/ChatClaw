package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino-ext/components/tool/browseruse"
	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
)

const browserUseCallTimeout = 60 * time.Second

const browserUseDescription = `
Interact with a web browser to perform various actions such as navigation, element interaction, content extraction, and tab management:
Navigation:
- 'go_to_url': Go to a specific URL in the current tab
- 'web_search': Search the query in the current tab, the query should be a search query like humans search in web.
Element Interaction:
- 'click_element': Click an element by index
- 'input_text': Input text into a form element
- 'scroll_down'/'scroll_up': Scroll the page (with optional pixel amount)
Content Extraction:
- 'extract_content': Extract page content to retrieve specific information from the page
Tab Management:
- 'switch_tab': Switch to a specific tab
- 'open_tab': Open a new tab with a URL
- 'close_tab': Close the current tab
Utility:
- 'wait': Wait for a specified number of seconds
`

// BrowserUseConfig defines configuration for the browser use tool.
type BrowserUseConfig struct {
	ExtractChatModel model.BaseChatModel
}

// browserUseTool defers Chrome launch until the LLM actually invokes the tool.
// It also enforces a per-call timeout to prevent hung operations.
type browserUseTool struct {
	config *BrowserUseConfig

	once  sync.Once
	inner *browseruse.Tool
	err   error
}

func (l *browserUseTool) init(ctx context.Context) (*browseruse.Tool, error) {
	l.once.Do(func() {
		var ddg duckduckgo.Search
		ddg, l.err = duckduckgo.NewSearch(ctx, &duckduckgo.Config{
			MaxResults: 5,
			Region:     duckduckgo.RegionWT,
		})
		if l.err != nil {
			return
		}

		cfg := &browseruse.Config{
			Headless:      true,
			DDGSearchTool: ddg,
		}
		if l.config != nil && l.config.ExtractChatModel != nil {
			cfg.ExtractChatModel = l.config.ExtractChatModel
		}

		l.inner, l.err = browseruse.NewBrowserUseTool(ctx, cfg)
	})
	return l.inner, l.err
}

// Info returns static tool metadata without starting the browser.
// The schema mirrors the upstream browseruse.Tool definition exactly.
func (l *browserUseTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ToolIDBrowserUse,
		Desc: browserUseDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(
			&jsonschema.Schema{
				Type: string(schema.Object),
				Properties: orderedmap.New[string, *jsonschema.Schema](
					orderedmap.WithInitialData[string, *jsonschema.Schema](
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key: "action",
							Value: &jsonschema.Schema{
								Type: string(schema.String),
								Enum: []any{
									"go_to_url", "click_element", "input_text",
									"scroll_down", "scroll_up", "web_search",
									"wait", "extract_content", "switch_tab",
									"open_tab", "close_tab",
								},
								Description: "The browser action to perform",
							},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "url",
							Value: &jsonschema.Schema{Type: string(schema.String), Description: "URL for 'go_to_url' or 'open_tab' actions"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "index",
							Value: &jsonschema.Schema{Type: string(schema.Integer), Description: "Element index for 'click_element', 'input_text' actions"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "text",
							Value: &jsonschema.Schema{Type: string(schema.String), Description: "Text for 'input_text' actions"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "scroll_amount",
							Value: &jsonschema.Schema{Type: string(schema.Integer), Description: "Pixels to scroll for 'scroll_down' or 'scroll_up' actions"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "tab_id",
							Value: &jsonschema.Schema{Type: string(schema.Integer), Description: "Tab ID for 'switch_tab' action"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "query",
							Value: &jsonschema.Schema{Type: string(schema.String), Description: "Search query for 'web_search' action"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "goal",
							Value: &jsonschema.Schema{Type: string(schema.String), Description: "Extraction goal for 'extract_content' action"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "keys",
							Value: &jsonschema.Schema{Type: string(schema.String), Description: "Keys to send for 'send_keys' action"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "seconds",
							Value: &jsonschema.Schema{Type: string(schema.Integer), Description: "Seconds to wait for 'wait' action"},
						},
					),
				),
			},
		),
	}, nil
}

// InvokableRun lazily starts Chrome on first call, then delegates with a per-call timeout.
func (l *browserUseTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	inner, err := l.init(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to initialize browser: %w", err)
	}

	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, e := inner.InvokableRun(ctx, argumentsInJSON, opts...)
		ch <- result{out, e}
	}()

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

// NewBrowserUseTool creates a lazy browser use tool.
// Chrome is NOT started until the LLM first invokes the tool.
func NewBrowserUseTool(_ context.Context, config *BrowserUseConfig) (tool.BaseTool, error) {
	return &browserUseTool{config: config}, nil
}
