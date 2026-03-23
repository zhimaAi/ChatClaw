# Scheduled Task History Iframe Design

**Date:** 2026-03-23

**Goal:** Render scheduled-task run conversations inside a real `iframe` so the embedded assistant view is fully isolated from the history dialog runtime, focus, and pointer behavior.

**Problem Summary**

The current history dialog mounts the full assistant page as a child Vue component:

- `TaskRunHistoryDialog.vue` renders `EmbeddedAssistantPage`
- `EmbeddedAssistantPage.vue` renders `AssistantPage.vue`
- `mode="embedded"` only hides parts of the page, but does not isolate:
  - Vue component lifecycle
  - Pinia stores
  - DOM focus management
  - global event subscriptions
  - pointer and layout behavior

That means the right panel is not behaving like an iframe. It is a live child page inside the same runtime tree, so it can interfere with:

- run list item click handling
- dialog close/focus behavior
- message sending state and model selection

## Chosen Approach

Create a dedicated frontend entry page for scheduled-task run history and load it with a real `iframe` from the history dialog.

This gives us:

- DOM isolation
- event isolation
- focus isolation
- layout isolation
- independent Vue app instance

The iframe content will still visually reuse assistant-related components where appropriate, but it must run as its own app entry instead of as a nested child component of the scheduled-task page.

## Rejected Alternatives

### 1. Keep extending `mode="embedded"`

Rejected because it does not create isolation. Even if we disable more features, it still shares the same runtime tree and remains vulnerable to modal interaction regressions.

### 2. Use in-app route and render inside the same SPA tree

Rejected because that still keeps the same app root alive unless we split routing and lifecycle carefully. It is more fragile than using a dedicated Vite entry.

## Target Architecture

### Parent side

`TaskRunHistoryDialog.vue` will:

- stop rendering `EmbeddedAssistantPage`
- render an `iframe`
- build a local URL pointing to a dedicated history embed page
- pass these query params:
  - `conversationId`
  - `agentId`
  - optional `runId`

The parent dialog remains the owner of:

- run list loading
- selected run switching
- dialog open/close state

The parent dialog does not need to coordinate assistant state with the iframe after initial URL creation.

### Iframe side

Add a new entry page, for example:

- `frontend/history-run.html`
- `frontend/src/history-run/main.ts`
- `frontend/src/history-run/App.vue`

This page will:

- read query params from `window.location`
- initialize Pinia and i18n like other standalone entries
- load the target conversation
- render a history-run assistant container

The iframe app should be intentionally narrow in scope:

- no top-level navigation
- no tab system integration
- no scheduled-task parent state access
- no parent dialog event wiring

## UI Behavior

### Visual behavior

The right panel should still look like a complete assistant page:

- messages area
- input area
- assistant styling
- loading and empty states

### Isolation behavior

The iframe must not affect:

- left-side run selection
- parent dialog close button
- parent dialog overlay/focus trap
- parent dialog pointer interactions

### Sending behavior

Inside the iframe, sending messages should be supported if the loaded conversation has a valid agent/model context.

Important rule:

- sending is local to the iframe app instance
- it must not mutate parent dialog state
- it must not re-open or re-focus the parent dialog

## Implementation Shape

### Shared assistant rendering

To avoid duplicating too much UI, extract a thin assistant-shell layer so the new iframe page can reuse:

- `ChatMessageList.vue`
- `ChatInputArea.vue`
- conversation loading helpers where safe

But do not directly mount the existing full `AssistantPage.vue` in the iframe. The full page contains:

- main/snap/embedded mode branching
- navigation-store behavior
- cross-tab events
- snap-mode logic
- agent sidebar and tab assumptions

Those are unnecessary for scheduled-task history and increase regression risk.

### New iframe page responsibilities

The new iframe page should have its own state for:

- active conversation id
- active agent id
- chat input
- model selection for the loaded conversation
- message loading and sending

It can reuse existing stores where needed, but only inside the iframe app root.

## URL Strategy

Use a dedicated Vite multi-page input.

Example target URL:

- `history-run.html?conversationId=123&agentId=5`

This matches the repo’s existing multi-entry structure (`index.html`, `winsnap.html`, `floatingball.html`, `selection.html`) and avoids retrofitting a router-only solution.

## Error Handling

The iframe app should explicitly handle:

- missing `conversationId`
- invalid numeric params
- missing conversation
- conversation message load failure
- send failure

Display these states inside the iframe content area without surfacing modal-level side effects to the parent.

## Testing Strategy

### Manual verification

1. Open scheduled task history.
2. Switch between multiple runs repeatedly.
3. Close the history dialog from:
   - close button
   - overlay click if supported
   - keyboard escape if supported
4. Re-open and verify the dialog only opens once.
5. Send a message in the iframe conversation.
6. Verify:
   - left run list still switches
   - parent dialog remains stable
   - no duplicate dialog appears

### Build verification

Run:

```powershell
pnpm --dir frontend exec vue-tsc
pnpm --dir frontend build:dev
```

Expected:

- TypeScript passes
- Vite multi-entry build includes the new history-run page

## Risks

### 1. Wails runtime inside iframe

The dedicated iframe page still needs Wails bindings/runtime to work correctly. We should verify the new entry can call the same generated bindings and runtime APIs when loaded as a local app page.

### 2. Shared store assumptions

Some existing assistant composables assume main-tab semantics. Reusing them blindly inside the iframe app can reintroduce side effects. The iframe app should depend on only the minimal assistant logic required for one conversation.

### 3. URL generation in packaged app

The parent must build an iframe URL that works both in dev and packaged Wails builds. The implementation should avoid hardcoded origins and use relative page URLs.

## Success Criteria

- The right panel is rendered through a real iframe, not a child assistant component.
- Switching run history items is stable.
- Closing the history dialog does not trigger a second dialog instance.
- The embedded assistant UI remains visually complete.
- Sending a message inside the iframe does not interfere with the parent history dialog.
