package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// maxSnapshotChars is the maximum character count for a snapshot text.
// Longer output is truncated with a hint for the LLM to scroll.
const maxSnapshotChars = 8000

// snapshotResult holds the formatted snapshot text and a flag indicating
// ref numbers were assigned via data-ref attributes in the DOM.
type snapshotResult struct {
	text     string
	maxRef   int  // highest ref number assigned
	hasRefs  bool // true if any refs were assigned
}

// jsElement is what the JS injection returns for each interactive element.
type jsElement struct {
	Ref         int    `json:"ref"`
	Tag         string `json:"tag"`
	Role        string `json:"role"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Type        string `json:"type"`
	Href        string `json:"href"`
	Placeholder string `json:"placeholder"`
	Disabled    bool   `json:"disabled"`
	Checked     bool   `json:"checked"`
	Focused     bool   `json:"focused"`
}

// getSnapshot injects data-ref attributes into interactive DOM elements and
// returns a structured text representation. Click/type actions later use
// the data-ref attribute to locate elements via CSS selector.
func (b *browserTool) getSnapshot(ctx context.Context) (*snapshotResult, error) {
	// Single JS call: find all interactive elements, assign data-ref, return descriptions.
	const script = `(() => {
    // Remove old data-ref attributes
    document.querySelectorAll('[data-ref]').forEach(el => el.removeAttribute('data-ref'));

    const selectors = 'a, button, input, select, textarea, [role="button"], [role="link"], ' +
        '[role="checkbox"], [role="radio"], [role="combobox"], [role="textbox"], [role="searchbox"], ' +
        '[role="menuitem"], [role="option"], [role="tab"], [role="switch"], [role="slider"], ' +
        '[tabindex], [contenteditable="true"], [onclick]';

    const els = document.querySelectorAll(selectors);
    const results = [];
    let ref = 1;

    for (const el of els) {
        // Skip invisible elements
        const rect = el.getBoundingClientRect();
        if (rect.width <= 0 || rect.height <= 0) continue;
        const style = getComputedStyle(el);
        if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') continue;

        // Skip elements fully outside viewport (with generous margin)
        // We still include them but they would be at the bottom

        // Assign data-ref for later targeting
        el.setAttribute('data-ref', String(ref));

        const tag = el.tagName.toLowerCase();
        const role = el.getAttribute('role') || '';
        let name = '';

        // Determine the display name
        if (tag === 'input' || tag === 'textarea') {
            name = el.getAttribute('aria-label') || el.getAttribute('placeholder') || el.getAttribute('name') || '';
        } else if (tag === 'select') {
            name = el.getAttribute('aria-label') || el.options?.[el.selectedIndex]?.text || '';
        } else if (tag === 'img') {
            name = el.getAttribute('alt') || '';
        } else {
            // Use innerText for other elements, truncated
            name = (el.innerText || el.textContent || '').trim();
            // Remove excessive whitespace
            name = name.replace(/\s+/g, ' ');
        }
        name = name.slice(0, 100);

        const value = (el.value !== undefined && el.value !== '' && tag !== 'a') ? String(el.value).slice(0, 80) : '';
        const href = (tag === 'a') ? (el.getAttribute('href') || '') : '';

        results.push({
            ref:         ref,
            tag:         tag,
            role:        role,
            name:        name,
            value:       value,
            type:        el.getAttribute('type') || '',
            href:        href,
            placeholder: el.getAttribute('placeholder') || '',
            disabled:    el.disabled || false,
            checked:     el.checked || false,
            focused:     document.activeElement === el
        });
        ref++;
    }
    return results;
})();`

	var elements []jsElement
	err := chromedp.Run(ctx, chromedp.Evaluate(script, &elements))
	if err != nil {
		return nil, fmt.Errorf("snapshot JS failed: %w", err)
	}

	return formatElements(elements), nil
}

// formatElements builds the snapshot text from JS-queried elements.
func formatElements(elements []jsElement) *snapshotResult {
	var sb strings.Builder
	maxRef := 0

	for _, el := range elements {
		role := inferRole(el)
		name := el.Name
		if name == "" && el.Placeholder != "" {
			name = el.Placeholder
		}
		if role == "link" && name == "" && el.Href != "" {
			name = el.Href
		}

		// Build line
		sb.WriteString(fmt.Sprintf("[ref=%d] %s", el.Ref, role))
		if name != "" {
			sb.WriteString(fmt.Sprintf(" %q", name))
		}
		if el.Value != "" {
			sb.WriteString(fmt.Sprintf(" value=%q", el.Value))
		}

		// States
		var states []string
		if el.Disabled {
			states = append(states, "disabled")
		}
		if el.Checked {
			states = append(states, "checked")
		}
		if el.Focused {
			states = append(states, "focused")
		}
		if len(states) > 0 {
			sb.WriteString(fmt.Sprintf(" (%s)", strings.Join(states, ", ")))
		}
		sb.WriteByte('\n')

		if el.Ref > maxRef {
			maxRef = el.Ref
		}
	}

	text := sb.String()
	text = truncateAtLine(text, maxSnapshotChars)

	return &snapshotResult{
		text:    text,
		maxRef:  maxRef,
		hasRefs: maxRef > 0,
	}
}

// inferRole returns a user-friendly role string for the element.
func inferRole(el jsElement) string {
	if el.Role != "" {
		return el.Role
	}
	switch el.Tag {
	case "a":
		return "link"
	case "button":
		return "button"
	case "input":
		switch el.Type {
		case "checkbox":
			return "checkbox"
		case "radio":
			return "radio"
		case "submit", "button", "reset":
			return "button"
		case "search":
			return "searchbox"
		default:
			return "textbox"
		}
	case "select":
		return "combobox"
	case "textarea":
		return "textbox"
	default:
		return el.Tag
	}
}

// truncateAtLine truncates text to maxLen, ensuring we don't cut in the middle of a line.
func truncateAtLine(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	cut := strings.LastIndex(text[:maxLen], "\n")
	if cut <= 0 {
		cut = maxLen
	}
	return text[:cut] + "\n... (content truncated, use scroll_down to see more)\n"
}

// resolveRef resolves a ref number to a CSS selector using the data-ref attribute.
func resolveRef(ref int) string {
	return fmt.Sprintf(`[data-ref="%d"]`, ref)
}

// clickByRef clicks an element by dispatching real CDP mouse events at the
// element's center coordinates. This is equivalent to a real user click and
// correctly triggers all browser behaviors: link navigation, JS event handlers,
// form submissions, target="_blank" new-tab opening, etc.
//
// Unlike JS el.click(), CDP mouse events go through the browser's input
// pipeline and are treated as trusted user interactions.
func (b *browserTool) clickByRef(ctx context.Context, ref int) error {
	selector := resolveRef(ref)

	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Step 1: Scroll element into view and get its center coordinates
		var raw json.RawMessage
		script := fmt.Sprintf(`(() => {
			const el = document.querySelector('%s');
			if (!el) return {error: 'element not found'};
			el.scrollIntoViewIfNeeded(true);
			const rect = el.getBoundingClientRect();
			return {x: rect.x + rect.width/2, y: rect.y + rect.height/2};
		})()`, selector)

		if err := chromedp.Evaluate(script, &raw).Do(ctx); err != nil {
			return fmt.Errorf("failed to locate element: %w", err)
		}

		var pos struct {
			X     float64 `json:"x"`
			Y     float64 `json:"y"`
			Error string  `json:"error"`
		}
		if err := json.Unmarshal(raw, &pos); err != nil {
			return fmt.Errorf("failed to parse position: %w", err)
		}
		if pos.Error != "" {
			return fmt.Errorf("ref %d: %s", ref, pos.Error)
		}

		// Step 2: Dispatch CDP mouse events (press + release) at the coordinates.
		// This is a trusted user input event that triggers all browser behaviors.
		if err := input.DispatchMouseEvent(input.MousePressed, pos.X, pos.Y).
			WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
			return fmt.Errorf("mouse press failed: %w", err)
		}
		if err := input.DispatchMouseEvent(input.MouseReleased, pos.X, pos.Y).
			WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
			return fmt.Errorf("mouse release failed: %w", err)
		}
		return nil
	}))
}

// typeByRef types text into an element identified by its ref number.
func (b *browserTool) typeByRef(ctx context.Context, ref int, text string) error {
	selector := resolveRef(ref)

	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Focus, clear, then type
		script := fmt.Sprintf(`(() => {
			const el = document.querySelector('%s');
			if (!el) return false;
			el.scrollIntoViewIfNeeded(true);
			el.focus();
			el.value = '';
			el.dispatchEvent(new Event('input', {bubbles: true}));
			return true;
		})()`, selector)

		var ok bool
		if err := chromedp.Evaluate(script, &ok).Do(ctx); err != nil {
			return fmt.Errorf("failed to focus element: %w", err)
		}
		if !ok {
			return fmt.Errorf("ref %d: element not found", ref)
		}

		// Type character by character for compatibility
		return chromedp.SendKeys(selector, text, chromedp.ByQuery).Do(ctx)
	}))
}
