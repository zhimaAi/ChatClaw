package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
)

const browserCallTimeout = 60 * time.Second

const browserToolDescription = `
Interact with a web browser to perform various actions such as navigation, element interaction, content extraction, and tab management.

Every action that changes the page automatically returns an accessibility snapshot — a compact, structured text representation of the current page with numbered ref IDs for interactive elements.

Navigation:
- 'go_to_url': Navigate to a URL. Returns page snapshot.
- 'web_search': Search a query via DuckDuckGo. Returns search results page snapshot.
Snapshot:
- 'snapshot': Get the current page snapshot without performing any action.
Element Interaction (use ref number from the snapshot):
- 'click': Click an element by its ref number. Returns updated snapshot.
- 'type': Clear a field and type text into it by ref number. Returns updated snapshot.
- 'scroll_down'/'scroll_up': Scroll the page (optional pixel amount). Returns updated snapshot.
Content Extraction:
- 'extract_content': Extract and summarize page content based on a goal.
Tab Management:
- 'switch_tab': Switch to a tab by index (0-based).
- 'open_tab': Open a new tab with a URL.
- 'close_tab': Close the current tab.
Utility:
- 'wait': Wait for a specified number of seconds.
`

// BrowserConfig defines configuration for the new browser tool.
type BrowserConfig struct {
	Headless         bool
	BrowserPath      string // optional; auto-detected if empty
	ExtractChatModel model.BaseChatModel
}

// browserTool implements tool.BaseTool (InvokableTool) using chromedp + DOM snapshot.
type browserTool struct {
	config *BrowserConfig

	once     sync.Once
	initErr  error
	allocCtx context.Context
	cancel   context.CancelFunc

	// Tab management
	mu        sync.Mutex
	tabCtxs   map[target.ID]context.Context
	tabCancel map[target.ID]context.CancelFunc
	activeTab target.ID

	// Last snapshot
	lastSnap *snapshotResult
}

// browserInput is the JSON schema the LLM sends when invoking the tool.
type browserInput struct {
	Action       string `json:"action"`
	URL          string `json:"url,omitempty"`
	Ref          int    `json:"ref,omitempty"`
	Text         string `json:"text,omitempty"`
	ScrollAmount int    `json:"scroll_amount,omitempty"`
	TabIndex     int    `json:"tab_index,omitempty"`
	Query        string `json:"query,omitempty"`
	Goal         string `json:"goal,omitempty"`
	Seconds      int    `json:"seconds,omitempty"`
}

// NewBrowserTool creates a lazy browser tool. Chrome is NOT started until first invocation.
func NewBrowserTool(_ context.Context, config *BrowserConfig) (tool.BaseTool, error) {
	if config == nil {
		config = &BrowserConfig{Headless: true}
	}
	return &browserTool{config: config}, nil
}

// init lazily starts Chrome on first call.
func (b *browserTool) init() error {
	b.once.Do(func() {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", b.config.Headless),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.Flag("disable-extensions", true),
			chromedp.WindowSize(1280, 1024),
		)

		// Use detected or configured browser path
		browserPath := b.config.BrowserPath
		if browserPath == "" {
			browserPath = detectBrowserPath()
		}
		if browserPath != "" {
			opts = append(opts, chromedp.ExecPath(browserPath))
		}

		b.allocCtx, b.cancel = chromedp.NewExecAllocator(context.Background(), opts...)

		// Create the first tab
		tabCtx, tabCancel := chromedp.NewContext(b.allocCtx)
		// Navigate to blank to ensure the browser actually starts
		if err := chromedp.Run(tabCtx, chromedp.Navigate("about:blank")); err != nil {
			b.initErr = fmt.Errorf("failed to start browser: %w", err)
			tabCancel()
			b.cancel()
			return
		}

		tid := chromedp.FromContext(tabCtx).Target.TargetID
		b.tabCtxs = map[target.ID]context.Context{tid: tabCtx}
		b.tabCancel = map[target.ID]context.CancelFunc{tid: tabCancel}
		b.activeTab = tid
	})
	return b.initErr
}

// activeCtx returns the chromedp context for the active tab.
func (b *browserTool) activeCtx() context.Context {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tabCtxs[b.activeTab]
}

// Info returns static tool metadata without starting the browser.
func (b *browserTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ToolIDBrowserUse,
		Desc: browserToolDescription,
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
									"snapshot", "go_to_url", "click", "type",
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
							Key:   "ref",
							Value: &jsonschema.Schema{Type: string(schema.Integer), Description: "Element ref number from the snapshot for 'click' and 'type' actions"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "text",
							Value: &jsonschema.Schema{Type: string(schema.String), Description: "Text to type for 'type' action"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "scroll_amount",
							Value: &jsonschema.Schema{Type: string(schema.Integer), Description: "Pixels to scroll (default 500) for 'scroll_down' or 'scroll_up'"},
						},
						orderedmap.Pair[string, *jsonschema.Schema]{
							Key:   "tab_index",
							Value: &jsonschema.Schema{Type: string(schema.Integer), Description: "Tab index (0-based) for 'switch_tab' action"},
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
							Key:   "seconds",
							Value: &jsonschema.Schema{Type: string(schema.Integer), Description: "Seconds to wait for 'wait' action"},
						},
					),
				),
			},
		),
	}, nil
}

// InvokableRun executes a browser action with a per-call timeout.
func (b *browserTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	if err := b.init(); err != nil {
		return "", fmt.Errorf("failed to initialize browser: %w", err)
	}

	var inp browserInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &inp); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, e := b.dispatch(ctx, &inp)
		ch <- result{out, e}
	}()

	timeoutCtx, cancel := context.WithTimeout(ctx, browserCallTimeout)
	defer cancel()

	select {
	case <-timeoutCtx.Done():
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("browser operation timed out after %v", browserCallTimeout)
	case r := <-ch:
		return r.output, r.err
	}
}

// dispatch routes to the appropriate action handler.
func (b *browserTool) dispatch(ctx context.Context, inp *browserInput) (string, error) {
	switch inp.Action {
	case "snapshot":
		return b.actionSnapshot(ctx)
	case "go_to_url":
		return b.actionGoToURL(ctx, inp.URL)
	case "click":
		return b.actionClick(ctx, inp.Ref)
	case "type":
		return b.actionType(ctx, inp.Ref, inp.Text)
	case "scroll_down":
		return b.actionScroll(ctx, inp.ScrollAmount, true)
	case "scroll_up":
		return b.actionScroll(ctx, inp.ScrollAmount, false)
	case "web_search":
		return b.actionWebSearch(ctx, inp.Query)
	case "wait":
		return b.actionWait(ctx, inp.Seconds)
	case "extract_content":
		return b.actionExtractContent(ctx, inp.Goal)
	case "switch_tab":
		return b.actionSwitchTab(ctx, inp.TabIndex)
	case "open_tab":
		return b.actionOpenTab(ctx, inp.URL)
	case "close_tab":
		return b.actionCloseTab(ctx)
	default:
		return "", fmt.Errorf("unknown action: %s", inp.Action)
	}
}

// --- Action implementations ---

func (b *browserTool) actionSnapshot(_ context.Context) (string, error) {
	snap, err := b.getSnapshot(b.activeCtx())
	if err != nil {
		return "", err
	}
	b.lastSnap = snap
	return b.snapshotWithURL()
}

func (b *browserTool) actionGoToURL(_ context.Context, url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("url is required for go_to_url action")
	}
	tabCtx := b.activeCtx()
	if err := chromedp.Run(tabCtx, chromedp.Navigate(url)); err != nil {
		return "", fmt.Errorf("navigation failed: %w", err)
	}
	// Wait for page to be ready
	if err := chromedp.Run(tabCtx, chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
		log.Printf("[browser] WaitReady after navigate: %v", err)
	}

	snap, err := b.getSnapshot(tabCtx)
	if err != nil {
		return fmt.Sprintf("Navigated to %s but snapshot failed: %v", url, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL()
}

func (b *browserTool) actionClick(_ context.Context, ref int) (string, error) {
	if b.lastSnap == nil || !b.lastSnap.hasRefs {
		return "", fmt.Errorf("no snapshot available; call snapshot first")
	}
	if ref < 1 || ref > b.lastSnap.maxRef {
		return "", fmt.Errorf("ref %d not found in current snapshot (valid range: 1-%d)", ref, b.lastSnap.maxRef)
	}

	tabCtx := b.activeCtx()

	// Get URL before click to detect navigation
	var urlBefore string
	_ = chromedp.Run(tabCtx, chromedp.Location(&urlBefore))

	// Register listener for new tabs that might open
	newTabCh := chromedp.WaitNewTarget(tabCtx, func(info *target.Info) bool {
		return info.Type == "page"
	})

	if err := b.clickByRef(tabCtx, ref); err != nil {
		return "", fmt.Errorf("click failed: %w", err)
	}

	// Check if a new tab was opened (give it a brief window)
	select {
	case newTargetID := <-newTabCh:
		// A new tab was opened — switch to it
		newTabCtx, newTabCancel := chromedp.NewContext(tabCtx, chromedp.WithTargetID(newTargetID))
		_ = chromedp.Run(newTabCtx, chromedp.WaitReady("body", chromedp.ByQuery))

		b.mu.Lock()
		b.tabCtxs[newTargetID] = newTabCtx
		b.tabCancel[newTargetID] = newTabCancel
		b.activeTab = newTargetID
		b.mu.Unlock()

		tabCtx = newTabCtx
	case <-time.After(500 * time.Millisecond):
		// No new tab — check if in-page navigation happened
	}

	// Wait for page to settle after navigation
	_ = chromedp.Run(tabCtx, chromedp.WaitReady("body", chromedp.ByQuery))

	// If URL changed, give extra time for dynamic content
	var urlAfter string
	_ = chromedp.Run(tabCtx, chromedp.Location(&urlAfter))
	if urlAfter != urlBefore {
		time.Sleep(500 * time.Millisecond)
	} else {
		time.Sleep(200 * time.Millisecond)
	}

	snap, err := b.getSnapshot(tabCtx)
	if err != nil {
		return fmt.Sprintf("Clicked ref %d but snapshot failed: %v", ref, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL()
}

func (b *browserTool) actionType(_ context.Context, ref int, text string) (string, error) {
	if b.lastSnap == nil || !b.lastSnap.hasRefs {
		return "", fmt.Errorf("no snapshot available; call snapshot first")
	}
	if ref < 1 || ref > b.lastSnap.maxRef {
		return "", fmt.Errorf("ref %d not found in current snapshot (valid range: 1-%d)", ref, b.lastSnap.maxRef)
	}

	tabCtx := b.activeCtx()

	if err := b.typeByRef(tabCtx, ref, text); err != nil {
		return "", fmt.Errorf("type failed: %w", err)
	}

	snap, err := b.getSnapshot(tabCtx)
	if err != nil {
		return fmt.Sprintf("Typed into ref %d but snapshot failed: %v", ref, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL()
}

func (b *browserTool) actionScroll(_ context.Context, amount int, down bool) (string, error) {
	if amount <= 0 {
		amount = 500
	}
	if !down {
		amount = -amount
	}

	tabCtx := b.activeCtx()
	err := chromedp.Run(tabCtx, chromedp.Evaluate(
		fmt.Sprintf("window.scrollBy(0, %d)", amount), nil,
	))
	if err != nil {
		return "", fmt.Errorf("scroll failed: %w", err)
	}

	// Brief wait for lazy-loaded content
	time.Sleep(200 * time.Millisecond)

	snap, err := b.getSnapshot(tabCtx)
	if err != nil {
		return fmt.Sprintf("Scrolled but snapshot failed: %v", err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL()
}

func (b *browserTool) actionWebSearch(ctx context.Context, query string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required for web_search action")
	}
	searchURL := "https://duckduckgo.com/?q=" + strings.ReplaceAll(query, " ", "+")
	return b.actionGoToURL(ctx, searchURL)
}

func (b *browserTool) actionWait(_ context.Context, seconds int) (string, error) {
	if seconds <= 0 {
		seconds = 1
	}
	if seconds > 30 {
		seconds = 30
	}
	time.Sleep(time.Duration(seconds) * time.Second)

	snap, err := b.getSnapshot(b.activeCtx())
	if err != nil {
		return fmt.Sprintf("Waited %d seconds but snapshot failed: %v", seconds, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL()
}

func (b *browserTool) actionExtractContent(_ context.Context, goal string) (string, error) {
	tabCtx := b.activeCtx()

	// Extract the page text content
	var textContent string
	err := chromedp.Run(tabCtx, chromedp.Evaluate(`document.body.innerText`, &textContent))
	if err != nil {
		return "", fmt.Errorf("failed to extract page text: %w", err)
	}

	// If we have a chat model, use it to extract/summarize
	if b.config != nil && b.config.ExtractChatModel != nil && goal != "" {
		extractPrompt := fmt.Sprintf(
			"Extract the following information from the web page content below.\n\nGoal: %s\n\nPage content:\n%s",
			goal, truncate(textContent, 6000),
		)
		resp, err := b.config.ExtractChatModel.Generate(context.Background(), []*schema.Message{
			{Role: schema.User, Content: extractPrompt},
		})
		if err != nil {
			return "", fmt.Errorf("extraction model failed: %w", err)
		}
		return resp.Content, nil
	}

	// No chat model: return raw truncated text
	return truncate(textContent, 4000), nil
}

func (b *browserTool) actionSwitchTab(_ context.Context, tabIndex int) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	tabs := b.tabList()
	if tabIndex < 0 || tabIndex >= len(tabs) {
		return "", fmt.Errorf("tab index %d out of range (have %d tabs)", tabIndex, len(tabs))
	}

	tid := tabs[tabIndex]
	b.activeTab = tid
	b.mu.Unlock()

	snap, err := b.getSnapshot(b.tabCtxs[tid])
	b.mu.Lock()
	if err != nil {
		return fmt.Sprintf("Switched to tab %d but snapshot failed: %v", tabIndex, err), nil
	}
	b.lastSnap = snap
	url := b.currentURL()
	return fmt.Sprintf("Switched to tab %d\nURL: %s\n\n%s", tabIndex, url, snap.text), nil
}

func (b *browserTool) actionOpenTab(_ context.Context, url string) (string, error) {
	if url == "" {
		url = "about:blank"
	}

	b.mu.Lock()
	tabCtx, tabCancel := chromedp.NewContext(b.allocCtx)
	b.mu.Unlock()

	if err := chromedp.Run(tabCtx, chromedp.Navigate(url)); err != nil {
		tabCancel()
		return "", fmt.Errorf("failed to open new tab: %w", err)
	}
	_ = chromedp.Run(tabCtx, chromedp.WaitReady("body", chromedp.ByQuery))

	tid := chromedp.FromContext(tabCtx).Target.TargetID
	b.mu.Lock()
	b.tabCtxs[tid] = tabCtx
	b.tabCancel[tid] = tabCancel
	b.activeTab = tid
	b.mu.Unlock()

	snap, err := b.getSnapshot(tabCtx)
	if err != nil {
		return fmt.Sprintf("Opened new tab with %s but snapshot failed: %v", url, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL()
}

func (b *browserTool) actionCloseTab(_ context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.tabCtxs) <= 1 {
		return "", fmt.Errorf("cannot close the last tab")
	}

	// Close the active tab
	if cancel, ok := b.tabCancel[b.activeTab]; ok {
		cancel()
	}
	delete(b.tabCtxs, b.activeTab)
	delete(b.tabCancel, b.activeTab)

	// Switch to the first remaining tab
	for tid := range b.tabCtxs {
		b.activeTab = tid
		break
	}

	b.mu.Unlock()
	snap, err := b.getSnapshot(b.tabCtxs[b.activeTab])
	b.mu.Lock()
	if err != nil {
		return "Closed tab, switched to another tab but snapshot failed", nil
	}
	b.lastSnap = snap
	url := b.currentURL()
	return fmt.Sprintf("Closed tab. Now on:\nURL: %s\n\n%s", url, snap.text), nil
}

// --- Helpers ---

// snapshotWithURL prepends the current URL to the snapshot text.
func (b *browserTool) snapshotWithURL() (string, error) {
	url := b.currentURL()
	return fmt.Sprintf("URL: %s\n\n%s", url, b.lastSnap.text), nil
}

// currentURL returns the current page URL.
func (b *browserTool) currentURL() string {
	var url string
	_ = chromedp.Run(b.activeCtx(), chromedp.Location(&url))
	return url
}

// tabList returns tab IDs in insertion order (best effort).
func (b *browserTool) tabList() []target.ID {
	tabs := make([]target.ID, 0, len(b.tabCtxs))
	for tid := range b.tabCtxs {
		tabs = append(tabs, tid)
	}
	return tabs
}

// truncate shortens s to at most maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
