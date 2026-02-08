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

	// opMu serializes browser operations. chromedp contexts and internal mutable state
	// are not designed for concurrent access.
	opMu sync.Mutex

	once     sync.Once
	initErr  error
	allocCtx context.Context
	cancel   context.CancelFunc

	// browserCtx is the first chromedp context created during init.
	// It holds a reference to the Browser instance and can be used for
	// browser-level CDP commands (like chromedp.Targets()) even when
	// individual tab targets have been destroyed by cross-origin navigation.
	browserCtx context.Context

	// Tab management
	mu        sync.Mutex
	tabCtxs   map[target.ID]context.Context
	tabCancel map[target.ID]context.CancelFunc
	tabOrder  []target.ID
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
		config = &BrowserConfig{Headless: false}
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
	tabCtx := b.activeCtx()
	opCtx, opCancel, stop := b.opContext(ctx, tabCtx)
	defer stop()
	defer opCancel()

	snap, err := b.getSnapshot(opCtx)
	if err != nil {
		return "", err
	}
	b.lastSnap = snap
	return b.snapshotWithURL(opCtx)
}

func (b *browserTool) actionGoToURL(ctx context.Context, urlStr string) (string, error) {
	if urlStr == "" {
		return "", fmt.Errorf("url is required for go_to_url action")
	}
	tabCtx := b.activeCtx()
	opCtx, opCancel, stop := b.opContext(ctx, tabCtx)
	defer stop()
	defer opCancel()

	// Step 1: Send the navigate command.
	if err := chromedp.Run(opCtx, chromedp.Navigate(urlStr)); err != nil {
		// Navigation itself failed — may be a cross-origin target swap.
		// Try to recover the target and retry.
		log.Printf("[browser] Navigate to %s failed: %v, attempting recovery...", urlStr, err)
		recoveredCtx, recErr := b.recoverActiveTarget(ctx)
		if recErr != nil {
			return "", fmt.Errorf("navigation failed: %w (recovery: %v)", err, recErr)
		}
		opCancel()
		stop()
		opCtx, opCancel, stop = b.opContext(ctx, recoveredCtx)
		defer stop()
		defer opCancel()

		// Retry navigate on the recovered target
		if err2 := chromedp.Run(opCtx, chromedp.Navigate(urlStr)); err2 != nil {
			return "", fmt.Errorf("navigation failed after recovery: %w", err2)
		}
	}

	// Step 2: Wait for the page body to be ready.
	if err := chromedp.Run(opCtx, chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
		log.Printf("[browser] WaitReady after navigate to %s failed: %v, attempting recovery...", urlStr, err)
		recoveredCtx, recErr := b.recoverActiveTarget(ctx)
		if recErr != nil {
			// Even recovery failed — try to snapshot anyway on whatever we have
			log.Printf("[browser] recovery after navigate WaitReady failed: %v", recErr)
		} else {
			opCancel()
			stop()
			opCtx, opCancel, stop = b.opContext(ctx, recoveredCtx)
			defer stop()
			defer opCancel()

			// Give the new target time to load
			_ = chromedp.Run(opCtx, chromedp.WaitReady("body", chromedp.ByQuery))
		}
	}

	// Step 3: Brief extra wait for dynamic content
	_ = chromedp.Run(opCtx, chromedp.Sleep(500*time.Millisecond))

	snap, err := b.getSnapshot(opCtx)
	if err != nil {
		return fmt.Sprintf("Navigated to %s but snapshot failed: %v", urlStr, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL(opCtx)
}

func (b *browserTool) actionClick(ctx context.Context, ref int) (string, error) {
	if b.lastSnap == nil || !b.lastSnap.hasRefs {
		return "", fmt.Errorf("no snapshot available; call snapshot first")
	}
	if ref < 1 || ref > b.lastSnap.maxRef {
		return "", fmt.Errorf("ref %d not found in current snapshot (valid range: 1-%d)", ref, b.lastSnap.maxRef)
	}

	// Check if the element has an href from the last snapshot. If so, navigate
	// directly instead of mouse-clicking. This avoids Chrome's cross-origin
	// process swap which destroys the old target on same-tab navigation.
	if b.lastSnap != nil && b.lastSnap.refHrefs != nil {
		if href, ok := b.lastSnap.refHrefs[ref]; ok && href != "" {
			log.Printf("[browser] ref %d has href, navigating to %s instead of clicking", ref, href)
			return b.actionGoToURL(ctx, href)
		}
	}

	// Non-link element: use real mouse click
	tabCtx := b.activeCtx()
	opCtx, opCancel, stop := b.opContext(ctx, tabCtx)
	defer stop()
	defer opCancel()

	// Get URL before click to detect navigation
	var urlBefore string
	_ = chromedp.Run(opCtx, chromedp.Location(&urlBefore))

	// Register listener for new tabs that might open
	newTabCh := chromedp.WaitNewTarget(opCtx, func(info *target.Info) bool {
		return info.Type == "page"
	})

	if err := b.clickByRef(opCtx, ref); err != nil {
		return "", fmt.Errorf("click failed: %w", err)
	}

	// Check if a new tab was opened (give it a brief window)
	var activeForSnapshot context.Context = tabCtx
	select {
	case newTargetID := <-newTabCh:
		// A new tab was opened — create a persistent tab context, then switch to it
		newTabCtx, newTabCancel := chromedp.NewContext(b.allocCtx, chromedp.WithTargetID(newTargetID))

		b.mu.Lock()
		b.tabCtxs[newTargetID] = newTabCtx
		b.tabCancel[newTargetID] = newTabCancel
		b.tabOrder = append(b.tabOrder, newTargetID)
		b.activeTab = newTargetID
		b.mu.Unlock()

		activeForSnapshot = newTabCtx
	case <-time.After(500 * time.Millisecond):
		// No new tab
	case <-ctx.Done():
		return "", ctx.Err()
	}

	// Wait for page to settle after click/navigation.
	snapTabCtx := activeForSnapshot
	snapOpCtx, snapCancel, snapStop := b.opContext(ctx, snapTabCtx)
	defer snapStop()
	defer snapCancel()

	// Try WaitReady. If it fails, the target may have been replaced by a
	// cross-origin navigation (Chrome Site Isolation). Attempt recovery.
	if err := chromedp.Run(snapOpCtx, chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
		log.Printf("[browser] WaitReady after click failed: %v, attempting target recovery...", err)

		recoveredCtx, recoveredErr := b.recoverActiveTarget(ctx)
		if recoveredErr != nil {
			log.Printf("[browser] target recovery failed: %v", recoveredErr)
			return fmt.Sprintf("Clicked ref %d but page navigation caused target loss: %v (recovery: %v)", ref, err, recoveredErr), nil
		}

		// Replace snapshot context with the recovered one
		snapCancel()
		snapStop()
		snapTabCtx = recoveredCtx
		snapOpCtx, snapCancel, snapStop = b.opContext(ctx, snapTabCtx)
		defer snapStop()
		defer snapCancel()

		// Wait for the recovered target to be ready
		if err2 := chromedp.Run(snapOpCtx, chromedp.WaitReady("body", chromedp.ByQuery)); err2 != nil {
			log.Printf("[browser] WaitReady after recovery: %v", err2)
		}
	}

	// If URL changed, give extra time for dynamic content
	var urlAfter string
	_ = chromedp.Run(snapOpCtx, chromedp.Location(&urlAfter))
	if urlAfter != "" && urlAfter != urlBefore {
		_ = chromedp.Run(snapOpCtx, chromedp.Sleep(1*time.Second))
	} else {
		_ = chromedp.Run(snapOpCtx, chromedp.Sleep(200*time.Millisecond))
	}

	snap, err := b.getSnapshot(snapOpCtx)
	if err != nil {
		return fmt.Sprintf("Clicked ref %d but snapshot failed: %v", ref, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL(snapOpCtx)
}

func (b *browserTool) actionType(ctx context.Context, ref int, text string) (string, error) {
	if b.lastSnap == nil || !b.lastSnap.hasRefs {
		return "", fmt.Errorf("no snapshot available; call snapshot first")
	}
	if ref < 1 || ref > b.lastSnap.maxRef {
		return "", fmt.Errorf("ref %d not found in current snapshot (valid range: 1-%d)", ref, b.lastSnap.maxRef)
	}

	tabCtx := b.activeCtx()
	opCtx, opCancel, stop := b.opContext(ctx, tabCtx)
	defer stop()
	defer opCancel()

	if err := b.typeByRef(opCtx, ref, text); err != nil {
		return "", fmt.Errorf("type failed: %w", err)
	}

	snap, err := b.getSnapshot(opCtx)
	if err != nil {
		return fmt.Sprintf("Typed into ref %d but snapshot failed: %v", ref, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL(opCtx)
}

func (b *browserTool) actionScroll(ctx context.Context, amount int, down bool) (string, error) {
	if amount <= 0 {
		amount = 500
	}
	if !down {
		amount = -amount
	}

	tabCtx := b.activeCtx()
	opCtx, opCancel, stop := b.opContext(ctx, tabCtx)
	defer stop()
	defer opCancel()

	err := chromedp.Run(opCtx, chromedp.Evaluate(
		fmt.Sprintf("window.scrollBy(0, %d)", amount), nil,
	))
	if err != nil {
		return "", fmt.Errorf("scroll failed: %w", err)
	}

	// Brief wait for lazy-loaded content
	_ = chromedp.Run(opCtx, chromedp.Sleep(200*time.Millisecond))

	snap, err := b.getSnapshot(opCtx)
	if err != nil {
		return fmt.Sprintf("Scrolled but snapshot failed: %v", err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL(opCtx)
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

	tabCtx := b.activeCtx()
	opCtx, opCancel, stop := b.opContext(ctx, tabCtx)
	defer stop()
	defer opCancel()

	snap, err := b.getSnapshot(opCtx)
	if err != nil {
		return fmt.Sprintf("Waited %d seconds but snapshot failed: %v", seconds, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL(opCtx)
}

func (b *browserTool) actionExtractContent(ctx context.Context, goal string) (string, error) {
	tabCtx := b.activeCtx()
	opCtx, opCancel, stop := b.opContext(ctx, tabCtx)
	defer stop()
	defer opCancel()

	// Extract the page text content
	var textContent string
	err := chromedp.Run(opCtx, chromedp.Evaluate(`document.body && document.body.innerText ? document.body.innerText : ''`, &textContent))
	if err != nil {
		return "", fmt.Errorf("failed to extract page text: %w", err)
	}

	// If we have a chat model, use it to extract/summarize
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

	// No chat model: return raw truncated text
	return truncate(textContent, 4000), nil
}

func (b *browserTool) actionSwitchTab(ctx context.Context, tabIndex int) (string, error) {
	b.mu.Lock()
	if tabIndex < 0 || tabIndex >= len(b.tabOrder) {
		have := len(b.tabOrder)
		b.mu.Unlock()
		return "", fmt.Errorf("tab index %d out of range (have %d tabs)", tabIndex, have)
	}
	tid := b.tabOrder[tabIndex]
	b.activeTab = tid
	tabCtx := b.tabCtxs[tid]
	b.mu.Unlock()

	opCtx, opCancel, stop := b.opContext(ctx, tabCtx)
	defer stop()
	defer opCancel()

	snap, err := b.getSnapshot(opCtx)
	if err != nil {
		return fmt.Sprintf("Switched to tab %d but snapshot failed: %v", tabIndex, err), nil
	}
	b.lastSnap = snap
	urlStr, _ := b.location(opCtx)
	return fmt.Sprintf("Switched to tab %d\nURL: %s\n\n%s", tabIndex, urlStr, snap.text), nil
}

func (b *browserTool) actionOpenTab(ctx context.Context, urlStr string) (string, error) {
	if urlStr == "" {
		urlStr = "about:blank"
	}

	tabCtx, tabCancel := chromedp.NewContext(b.allocCtx)
	tid := chromedp.FromContext(tabCtx).Target.TargetID

	b.mu.Lock()
	if b.tabCtxs == nil {
		b.tabCtxs = make(map[target.ID]context.Context)
	}
	if b.tabCancel == nil {
		b.tabCancel = make(map[target.ID]context.CancelFunc)
	}
	b.tabCtxs[tid] = tabCtx
	b.tabCancel[tid] = tabCancel
	b.tabOrder = append(b.tabOrder, tid)
	b.activeTab = tid
	b.mu.Unlock()

	opCtx, opCancel, stop := b.opContext(ctx, tabCtx)
	defer stop()
	defer opCancel()

	if err := chromedp.Run(opCtx, chromedp.Navigate(urlStr), chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
		// If open failed, clean up the tab we just created.
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

	snap, err := b.getSnapshot(opCtx)
	if err != nil {
		return fmt.Sprintf("Opened new tab with %s but snapshot failed: %v", urlStr, err), nil
	}
	b.lastSnap = snap
	return b.snapshotWithURL(opCtx)
}

func (b *browserTool) actionCloseTab(ctx context.Context) (string, error) {
	b.mu.Lock()
	if len(b.tabOrder) <= 1 {
		b.mu.Unlock()
		return "", fmt.Errorf("cannot close the last tab")
	}

	closing := b.activeTab
	closingIdx := indexOfTargetID(b.tabOrder, closing)

	// Cancel and remove closing tab.
	// Do not cancel browserCtx — it holds the Browser WebSocket connection.
	if closingCtx, ok := b.tabCtxs[closing]; ok {
		if closingCtx != b.browserCtx {
			if cancel, cancelOk := b.tabCancel[closing]; cancelOk {
				cancel()
			}
		}
	}
	delete(b.tabCtxs, closing)
	delete(b.tabCancel, closing)
	b.tabOrder = removeTargetID(b.tabOrder, closing)

	// Select next active tab
	nextIdx := closingIdx - 1
	if nextIdx < 0 || nextIdx >= len(b.tabOrder) {
		nextIdx = 0
	}
	b.activeTab = b.tabOrder[nextIdx]
	nextTabCtx := b.tabCtxs[b.activeTab]
	b.mu.Unlock()

	opCtx, opCancel, stop := b.opContext(ctx, nextTabCtx)
	defer stop()
	defer opCancel()

	snap, err := b.getSnapshot(opCtx)
	if err != nil {
		return "Closed tab, switched to another tab but snapshot failed", nil
	}
	b.lastSnap = snap
	urlStr, _ := b.location(opCtx)
	return fmt.Sprintf("Closed tab. Now on:\nURL: %s\n\n%s", urlStr, snap.text), nil
}

// --- Target Recovery ---

// recoverActiveTarget finds the current active page target after a cross-origin
// navigation destroyed the old target (Chrome Site Isolation / process swap).
//
// It uses b.browserCtx (which holds the Browser instance from init) to call
// chromedp.Targets(), finds the replacement target, and updates tab management.
func (b *browserTool) recoverActiveTarget(ctx context.Context) (context.Context, error) {
	// b.browserCtx holds the Browser instance. chromedp.Targets() needs a
	// context that went through NewContext (not just the allocator context).
	targets, err := chromedp.Targets(b.browserCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}

	b.mu.Lock()
	oldActiveTab := b.activeTab
	knownTargets := make(map[target.ID]bool, len(b.tabCtxs))
	for tid := range b.tabCtxs {
		knownTargets[tid] = true
	}
	b.mu.Unlock()

	// First pass: prefer a page target we haven't seen (the replacement).
	var newTargetID target.ID
	for _, t := range targets {
		if t.Type != "page" || t.URL == "about:blank" || t.URL == "" {
			continue
		}
		if !knownTargets[t.TargetID] {
			newTargetID = t.TargetID
			break
		}
	}

	// Second pass: pick any page target that is not our dead active tab.
	if newTargetID == "" {
		for _, t := range targets {
			if t.Type == "page" && t.URL != "about:blank" && t.URL != "" && t.TargetID != oldActiveTab {
				newTargetID = t.TargetID
				break
			}
		}
	}

	if newTargetID == "" {
		return nil, fmt.Errorf("no suitable page target found for recovery (saw %d targets)", len(targets))
	}

	log.Printf("[browser] recovering target: old=%s new=%s", oldActiveTab, newTargetID)

	// Create a new chromedp context attached to the replacement target
	newTabCtx, newTabCancel := chromedp.NewContext(b.allocCtx, chromedp.WithTargetID(newTargetID))

	// Swap old → new in tab management.
	// IMPORTANT: Do NOT cancel the old tab context if it is browserCtx,
	// because browserCtx holds the Browser WebSocket connection we need alive.
	b.mu.Lock()
	if oldCtx, ok := b.tabCtxs[oldActiveTab]; ok {
		if oldCtx != b.browserCtx {
			if cancel, cancelOk := b.tabCancel[oldActiveTab]; cancelOk {
				cancel()
			}
		}
	}
	delete(b.tabCtxs, oldActiveTab)
	delete(b.tabCancel, oldActiveTab)

	for i, tid := range b.tabOrder {
		if tid == oldActiveTab {
			b.tabOrder[i] = newTargetID
			break
		}
	}

	b.tabCtxs[newTargetID] = newTabCtx
	b.tabCancel[newTargetID] = newTabCancel
	b.activeTab = newTargetID
	b.mu.Unlock()

	return newTabCtx, nil
}

// --- Helpers ---

// snapshotWithURL prepends the current URL to the snapshot text.
func (b *browserTool) snapshotWithURL(ctx context.Context) (string, error) {
	urlStr, _ := b.location(ctx)
	if b.lastSnap == nil {
		return fmt.Sprintf("URL: %s\n\n(no snapshot)", urlStr), nil
	}
	return fmt.Sprintf("URL: %s\n\n%s", urlStr, b.lastSnap.text), nil
}

// location returns the current page URL in the provided chromedp context.
func (b *browserTool) location(ctx context.Context) (string, error) {
	var urlStr string
	if err := chromedp.Run(ctx, chromedp.Location(&urlStr)); err != nil {
		return "", err
	}
	return urlStr, nil
}

// truncate shortens s to at most maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

// opContext creates a cancellable chromedp context derived from a tab context.
// It will be cancelled automatically when the request context is done.
func (b *browserTool) opContext(reqCtx context.Context, tabCtx context.Context) (context.Context, context.CancelFunc, func() bool) {
	opCtx, cancel := context.WithCancel(tabCtx)
	stop := context.AfterFunc(reqCtx, cancel)
	return opCtx, cancel, stop
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
