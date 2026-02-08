package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
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

// Closeable is an optional interface that tools can implement to release
// resources (e.g., browser processes) when they are replaced or removed.
type Closeable interface {
	Close()
}

const browserToolDescription = `
Interact with a web browser to perform various actions such as navigation, element interaction, content extraction, and tab management.

Every action that changes the page automatically returns a page snapshot — a compact, structured text representation of the current page with numbered ref IDs for interactive elements.

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

	// opMu serializes browser operations. chromedp contexts and internal mutable
	// state are not designed for concurrent access from different goroutines.
	opMu sync.Mutex

	once     sync.Once
	initErr  error
	allocCtx context.Context
	cancel   context.CancelFunc
	closed   bool // set by Close(); prevents further init

	// browserCtx is the first chromedp context created during init.
	// It holds the Browser WebSocket connection and can be used for
	// browser-level CDP commands (like chromedp.Targets()) even when
	// individual tab targets have been destroyed by cross-origin navigation.
	// This context must NEVER be cancelled while the browser is alive.
	browserCtx context.Context

	// Tab management
	mu        sync.Mutex
	tabCtxs   map[target.ID]context.Context
	tabCancel map[target.ID]context.CancelFunc
	tabOrder  []target.ID
	activeTab target.ID

	// Last snapshot (per-instance, safe because opMu serializes calls)
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

// BrowserTool is the public handle returned by NewBrowserTool.
// It implements tool.BaseTool and also exposes Close() for callers that
// manage the browser lifecycle explicitly (e.g., per-conversation instances).
type BrowserTool = browserTool

// NewBrowserTool creates a lazy browser tool. Chrome is NOT started until first invocation.
func NewBrowserTool(_ context.Context, config *BrowserConfig) (*BrowserTool, error) {
	if config == nil {
		config = &BrowserConfig{Headless: false}
	}
	return &browserTool{config: config}, nil
}

// init lazily starts Chrome on first call.
func (b *browserTool) init() error {
	if b.closed {
		return fmt.Errorf("browser tool has been closed")
	}
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

		// NOTE: We intentionally use Background() here to keep the browser process
		// alive across tool calls. Per-call timeouts/cancellation are handled in InvokableRun.
		b.allocCtx, b.cancel = chromedp.NewExecAllocator(context.Background(), opts...)

		// Create the first tab.
		tabCtx, tabCancel := chromedp.NewContext(b.allocCtx)
		// Navigate to blank to ensure the browser actually starts
		if err := chromedp.Run(tabCtx, chromedp.Navigate("about:blank")); err != nil {
			b.initErr = fmt.Errorf("failed to start browser: %w", err)
			tabCancel()
			b.cancel()
			return
		}

		// Save this context for browser-level CDP commands (chromedp.Targets).
		// The Browser WebSocket connection lives on this context and survives
		// individual tab target destruction. We must NEVER cancel this context.
		b.browserCtx = tabCtx

		tid := chromedp.FromContext(tabCtx).Target.TargetID
		b.tabCtxs = map[target.ID]context.Context{tid: tabCtx}
		b.tabCancel = map[target.ID]context.CancelFunc{tid: tabCancel}
		b.tabOrder = []target.ID{tid}
		b.activeTab = tid
	})
	return b.initErr
}

// Close shuts down the Chrome process and releases all resources.
// Implements the Closeable interface so the ToolRegistry can clean up
// when a tool is replaced or removed.
func (b *browserTool) Close() {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	if b.cancel != nil {
		b.cancel()
	}
	b.closed = true
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

	// Serialize all operations to avoid concurrent chromedp + state access.
	b.opMu.Lock()
	defer b.opMu.Unlock()

	var inp browserInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &inp); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	inp.Action = strings.ToLower(strings.TrimSpace(inp.Action))

	timeoutCtx, cancel := context.WithTimeout(ctx, browserCallTimeout)
	defer cancel()
	out, err := b.dispatch(timeoutCtx, &inp)
	if err != nil {
		return "", err
	}
	return out, nil
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

func (b *browserTool) actionSnapshot(ctx context.Context) (string, error) {
	opCtx := b.makeOpCtx(ctx)
	defer opCtx.cancel()
	return b.takeSnapshot(opCtx.ctx, "")
}

func (b *browserTool) actionGoToURL(ctx context.Context, urlStr string) (string, error) {
	if urlStr == "" {
		return "", fmt.Errorf("url is required for go_to_url action")
	}

	opCtx := b.makeOpCtx(ctx)
	defer opCtx.cancel()

	if err := chromedp.Run(opCtx.ctx, chromedp.Navigate(urlStr)); err != nil {
		// Cross-origin target swap may destroy the old target. Try recovery.
		log.Printf("[browser] Navigate to %s failed: %v, recovering...", urlStr, err)
		if recErr := b.recoverOp(ctx, opCtx); recErr != nil {
			return "", fmt.Errorf("navigation failed: %w (recovery: %v)", err, recErr)
		}
		if err2 := chromedp.Run(opCtx.ctx, chromedp.Navigate(urlStr)); err2 != nil {
			return "", fmt.Errorf("navigation failed after recovery: %w", err2)
		}
	}

	b.waitPageReady(ctx, opCtx)
	_ = chromedp.Run(opCtx.ctx, chromedp.Sleep(500*time.Millisecond))
	return b.takeSnapshot(opCtx.ctx, fmt.Sprintf("Navigated to %s", urlStr))
}

func (b *browserTool) actionClick(ctx context.Context, ref int) (string, error) {
	if b.lastSnap == nil || !b.lastSnap.hasRefs {
		return "", fmt.Errorf("no snapshot available; call snapshot first")
	}
	if ref < 1 || ref > b.lastSnap.maxRef {
		return "", fmt.Errorf("ref %d not found in current snapshot (valid range: 1-%d)", ref, b.lastSnap.maxRef)
	}

	// For <a href="..."> links, navigate directly to avoid Chrome cross-origin
	// target swap that destroys the old target on same-tab navigation.
	if href, ok := b.lastSnap.refHrefs[ref]; ok && href != "" {
		log.Printf("[browser] ref %d has href, navigating to %s instead of clicking", ref, href)
		return b.actionGoToURL(ctx, href)
	}

	// Non-link element: use real mouse click.
	opCtx := b.makeOpCtx(ctx)
	defer opCtx.cancel()

	var urlBefore string
	_ = chromedp.Run(opCtx.ctx, chromedp.Location(&urlBefore))

	// Listen for new-tab events before clicking.
	newTabCh := chromedp.WaitNewTarget(opCtx.ctx, func(info *target.Info) bool {
		return info.Type == "page"
	})

	if err := b.clickByRef(opCtx.ctx, ref); err != nil {
		return "", fmt.Errorf("click failed: %w", err)
	}

	// Wait briefly for a potential new tab.
	select {
	case newTargetID := <-newTabCh:
		b.adoptNewTab(newTargetID)
		opCtx.switchTab(b.activeCtx())
	case <-time.After(500 * time.Millisecond):
		// No new tab opened.
	case <-ctx.Done():
		return "", ctx.Err()
	}

	// Wait for page to settle. Recovery handles cross-origin target loss.
	b.waitPageReady(ctx, opCtx)

	// Extra wait if the URL changed (dynamic content loading).
	var urlAfter string
	_ = chromedp.Run(opCtx.ctx, chromedp.Location(&urlAfter))
	if urlAfter != "" && urlAfter != urlBefore {
		_ = chromedp.Run(opCtx.ctx, chromedp.Sleep(1*time.Second))
	} else {
		_ = chromedp.Run(opCtx.ctx, chromedp.Sleep(200*time.Millisecond))
	}

	return b.takeSnapshot(opCtx.ctx, fmt.Sprintf("Clicked ref %d", ref))
}

func (b *browserTool) actionType(ctx context.Context, ref int, text string) (string, error) {
	if b.lastSnap == nil || !b.lastSnap.hasRefs {
		return "", fmt.Errorf("no snapshot available; call snapshot first")
	}
	if ref < 1 || ref > b.lastSnap.maxRef {
		return "", fmt.Errorf("ref %d not found in current snapshot (valid range: 1-%d)", ref, b.lastSnap.maxRef)
	}

	opCtx := b.makeOpCtx(ctx)
	defer opCtx.cancel()

	if err := b.typeByRef(opCtx.ctx, ref, text); err != nil {
		return "", fmt.Errorf("type failed: %w", err)
	}
	return b.takeSnapshot(opCtx.ctx, fmt.Sprintf("Typed into ref %d", ref))
}

func (b *browserTool) actionScroll(ctx context.Context, amount int, down bool) (string, error) {
	if amount <= 0 {
		amount = 500
	}
	if !down {
		amount = -amount
	}

	opCtx := b.makeOpCtx(ctx)
	defer opCtx.cancel()

	if err := chromedp.Run(opCtx.ctx, chromedp.Evaluate(
		fmt.Sprintf("window.scrollBy(0, %d)", amount), nil,
	)); err != nil {
		return "", fmt.Errorf("scroll failed: %w", err)
	}
	_ = chromedp.Run(opCtx.ctx, chromedp.Sleep(200*time.Millisecond))
	return b.takeSnapshot(opCtx.ctx, "Scrolled")
}

func (b *browserTool) actionWebSearch(ctx context.Context, query string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required for web_search action")
	}
	searchURL := "https://duckduckgo.com/?q=" + url.QueryEscape(query)
	return b.actionGoToURL(ctx, searchURL)
}

func (b *browserTool) actionWait(ctx context.Context, seconds int) (string, error) {
	if seconds <= 0 {
		seconds = 1
	}
	if seconds > 30 {
		seconds = 30
	}
	if err := sleepWithContext(ctx, time.Duration(seconds)*time.Second); err != nil {
		return "", err
	}
	opCtx := b.makeOpCtx(ctx)
	defer opCtx.cancel()
	return b.takeSnapshot(opCtx.ctx, fmt.Sprintf("Waited %d seconds", seconds))
}

func (b *browserTool) actionExtractContent(ctx context.Context, goal string) (string, error) {
	opCtx := b.makeOpCtx(ctx)
	defer opCtx.cancel()

	var textContent string
	if err := chromedp.Run(opCtx.ctx, chromedp.Evaluate(
		`document.body && document.body.innerText ? document.body.innerText : ''`, &textContent,
	)); err != nil {
		return "", fmt.Errorf("failed to extract page text: %w", err)
	}

	// Use the chat model for extraction if available.
	if b.config != nil && b.config.ExtractChatModel != nil && goal != "" {
		extractPrompt := fmt.Sprintf(
			"Extract the following information from the web page content below.\n\nGoal: %s\n\nPage content:\n%s",
			goal, truncate(textContent, 6000),
		)
		resp, err := b.config.ExtractChatModel.Generate(ctx, []*schema.Message{
			{Role: schema.User, Content: extractPrompt},
		})
		if err != nil {
			return "", fmt.Errorf("extraction model failed: %w", err)
		}
		return resp.Content, nil
	}
	return truncate(textContent, 4000), nil
}

func (b *browserTool) actionSwitchTab(ctx context.Context, tabIndex int) (string, error) {
	b.mu.Lock()
	if tabIndex < 0 || tabIndex >= len(b.tabOrder) {
		have := len(b.tabOrder)
		b.mu.Unlock()
		return "", fmt.Errorf("tab index %d out of range (have %d tabs)", tabIndex, have)
	}
	b.activeTab = b.tabOrder[tabIndex]
	b.mu.Unlock()

	opCtx := b.makeOpCtx(ctx)
	defer opCtx.cancel()
	return b.takeSnapshot(opCtx.ctx, fmt.Sprintf("Switched to tab %d", tabIndex))
}

func (b *browserTool) actionOpenTab(ctx context.Context, urlStr string) (string, error) {
	if urlStr == "" {
		urlStr = "about:blank"
	}

	tabCtx, tabCancel := chromedp.NewContext(b.allocCtx)
	tid := chromedp.FromContext(tabCtx).Target.TargetID

	b.mu.Lock()
	b.tabCtxs[tid] = tabCtx
	b.tabCancel[tid] = tabCancel
	b.tabOrder = append(b.tabOrder, tid)
	b.activeTab = tid
	b.mu.Unlock()

	opCtx := b.makeOpCtxFrom(ctx, tabCtx)
	defer opCtx.cancel()

	if err := chromedp.Run(opCtx.ctx, chromedp.Navigate(urlStr), chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
		// Clean up the failed tab.
		b.mu.Lock()
		delete(b.tabCtxs, tid)
		delete(b.tabCancel, tid)
		b.tabOrder = removeTargetID(b.tabOrder, tid)
		if b.activeTab == tid && len(b.tabOrder) > 0 {
			b.activeTab = b.tabOrder[0]
		}
		b.mu.Unlock()
		tabCancel()
		return "", fmt.Errorf("failed to open new tab: %w", err)
	}
	return b.takeSnapshot(opCtx.ctx, fmt.Sprintf("Opened new tab with %s", urlStr))
}

func (b *browserTool) actionCloseTab(ctx context.Context) (string, error) {
	b.mu.Lock()
	if len(b.tabOrder) <= 1 {
		b.mu.Unlock()
		return "", fmt.Errorf("cannot close the last tab")
	}

	closing := b.activeTab
	closingIdx := indexOfTargetID(b.tabOrder, closing)

	// Cancel and remove the closing tab.
	// Never cancel browserCtx — it holds the Browser WebSocket connection.
	b.cancelTabLocked(closing)
	delete(b.tabCtxs, closing)
	delete(b.tabCancel, closing)
	b.tabOrder = removeTargetID(b.tabOrder, closing)

	nextIdx := closingIdx - 1
	if nextIdx < 0 || nextIdx >= len(b.tabOrder) {
		nextIdx = 0
	}
	b.activeTab = b.tabOrder[nextIdx]
	b.mu.Unlock()

	opCtx := b.makeOpCtx(ctx)
	defer opCtx.cancel()
	return b.takeSnapshot(opCtx.ctx, "Closed tab")
}

// ---------------------------------------------------------------------------
// opHandle: a small wrapper around a cancellable operation context.
// It can be reassigned (e.g. after target recovery) without leaking the old
// cancel/stop resources.
// ---------------------------------------------------------------------------

type opHandle struct {
	ctx    context.Context
	stopFn func() bool          // returned by context.AfterFunc
	ccFn   context.CancelFunc   // cancel function for ctx
}

// cancel releases all resources held by this handle.
func (h *opHandle) cancel() {
	if h.stopFn != nil {
		h.stopFn()
	}
	if h.ccFn != nil {
		h.ccFn()
	}
}

// switchTab reassigns the handle to a different tab context, cleaning up old resources.
func (h *opHandle) switchTab(tabCtx context.Context) {
	h.cancel()
	h.ctx, h.ccFn = context.WithCancel(tabCtx)
	// No AfterFunc needed — the outer reqCtx lifetime is shorter than tabCtx.
}

// makeOpCtx creates an opHandle derived from the current active tab, cancelled
// when reqCtx expires.
func (b *browserTool) makeOpCtx(reqCtx context.Context) *opHandle {
	return b.makeOpCtxFrom(reqCtx, b.activeCtx())
}

// makeOpCtxFrom creates an opHandle from an explicit tab context.
func (b *browserTool) makeOpCtxFrom(reqCtx context.Context, tabCtx context.Context) *opHandle {
	opCtx, ccFn := context.WithCancel(tabCtx)
	stopFn := context.AfterFunc(reqCtx, ccFn)
	return &opHandle{ctx: opCtx, stopFn: stopFn, ccFn: ccFn}
}

// ---------------------------------------------------------------------------
// Target recovery: find the replacement page target after a cross-origin
// navigation destroyed the old one (Chrome Site Isolation / process swap).
// ---------------------------------------------------------------------------

// recoverActiveTarget discovers the new page target via b.browserCtx and
// updates tab management state.
func (b *browserTool) recoverActiveTarget() (context.Context, error) {
	targets, err := chromedp.Targets(b.browserCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}

	b.mu.Lock()
	oldActive := b.activeTab
	known := make(map[target.ID]bool, len(b.tabCtxs))
	for tid := range b.tabCtxs {
		known[tid] = true
	}
	b.mu.Unlock()

	// Prefer a page target we haven't seen yet (the replacement).
	var newID target.ID
	for _, t := range targets {
		if t.Type == "page" && t.URL != "about:blank" && t.URL != "" && !known[t.TargetID] {
			newID = t.TargetID
			break
		}
	}
	// Fallback: any page target that is not the dead one.
	if newID == "" {
		for _, t := range targets {
			if t.Type == "page" && t.URL != "about:blank" && t.URL != "" && t.TargetID != oldActive {
				newID = t.TargetID
				break
			}
		}
	}
	if newID == "" {
		return nil, fmt.Errorf("no suitable page target found for recovery (saw %d targets)", len(targets))
	}

	log.Printf("[browser] recovering target: old=%s new=%s", oldActive, newID)

	newTabCtx, newTabCancel := chromedp.NewContext(b.allocCtx, chromedp.WithTargetID(newID))

	b.mu.Lock()
	b.cancelTabLocked(oldActive)
	delete(b.tabCtxs, oldActive)
	delete(b.tabCancel, oldActive)
	for i, tid := range b.tabOrder {
		if tid == oldActive {
			b.tabOrder[i] = newID
			break
		}
	}
	b.tabCtxs[newID] = newTabCtx
	b.tabCancel[newID] = newTabCancel
	b.activeTab = newID
	b.mu.Unlock()

	return newTabCtx, nil
}

// recoverOp runs recoverActiveTarget and reassigns the opHandle to the
// recovered tab context. Returns an error if recovery fails.
func (b *browserTool) recoverOp(reqCtx context.Context, h *opHandle) error {
	recovered, err := b.recoverActiveTarget()
	if err != nil {
		return err
	}
	h.cancel()
	h.ctx, h.ccFn = context.WithCancel(recovered)
	h.stopFn = context.AfterFunc(reqCtx, h.ccFn)
	return nil
}

// waitPageReady waits for the page body to become ready. If the wait fails
// (e.g. cross-origin target swap), it attempts target recovery transparently.
func (b *browserTool) waitPageReady(reqCtx context.Context, h *opHandle) {
	if err := chromedp.Run(h.ctx, chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
		log.Printf("[browser] WaitReady failed: %v, attempting recovery...", err)
		if recErr := b.recoverOp(reqCtx, h); recErr != nil {
			log.Printf("[browser] recovery failed: %v", recErr)
			return
		}
		_ = chromedp.Run(h.ctx, chromedp.WaitReady("body", chromedp.ByQuery))
	}
}

// adoptNewTab registers a newly opened tab and switches to it.
func (b *browserTool) adoptNewTab(id target.ID) {
	newCtx, newCancel := chromedp.NewContext(b.allocCtx, chromedp.WithTargetID(id))
	b.mu.Lock()
	b.tabCtxs[id] = newCtx
	b.tabCancel[id] = newCancel
	b.tabOrder = append(b.tabOrder, id)
	b.activeTab = id
	b.mu.Unlock()
}

// cancelTabLocked cancels a tab context. It skips browserCtx to preserve the
// Browser WebSocket connection. Must be called with b.mu held.
func (b *browserTool) cancelTabLocked(tid target.ID) {
	tabCtx, ok := b.tabCtxs[tid]
	if !ok {
		return
	}
	if tabCtx == b.browserCtx {
		return // never cancel the browser-level context
	}
	if cancel, cancelOk := b.tabCancel[tid]; cancelOk {
		cancel()
	}
}

// ---------------------------------------------------------------------------
// Common helpers
// ---------------------------------------------------------------------------

// takeSnapshot captures a DOM snapshot, stores it in b.lastSnap, and returns
// the snapshot text prefixed with the current URL. The hint is prepended when
// the snapshot fails (e.g. "Navigated to ..." or "Clicked ref 3").
func (b *browserTool) takeSnapshot(ctx context.Context, hint string) (string, error) {
	snap, err := b.getSnapshot(ctx)
	if err != nil {
		if hint != "" {
			return fmt.Sprintf("%s but snapshot failed: %v", hint, err), nil
		}
		return "", err
	}
	b.lastSnap = snap

	var urlStr string
	_ = chromedp.Run(ctx, chromedp.Location(&urlStr))

	return fmt.Sprintf("URL: %s\n\n%s", urlStr, snap.text), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func indexOfTargetID(ids []target.ID, id target.ID) int {
	for i := range ids {
		if ids[i] == id {
			return i
		}
	}
	return -1
}

func removeTargetID(ids []target.ID, id target.ID) []target.ID {
	out := make([]target.ID, 0, len(ids))
	for _, v := range ids {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}
