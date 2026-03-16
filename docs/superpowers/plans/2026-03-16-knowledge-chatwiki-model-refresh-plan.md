# Knowledge ChatWiki Model Refresh Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure Knowledge page model selector refreshes and includes ChatWiki models by reacting to model list changes and tab activation.

**Architecture:** Reuse the existing `loadModels()` from `useModelSelection()` in KnowledgePage. Add an event subscription to `models:changed` and refresh on tab activation. Clean up the subscription on unmount.

**Tech Stack:** Vue 3 + TypeScript, Wails runtime Events

---

## File Structure
- Modify: `frontend/src/pages/knowledge/KnowledgePage.vue`
  - Responsibility: Knowledge page state, model selection, and chat input wiring.
  - Change: add model refresh hooks (events + tab activation).

## Chunk 1: Model Refresh Hooks

### Task 1: Subscribe to models change and refresh on tab activation

**Files:**
- Modify: `frontend/src/pages/knowledge/KnowledgePage.vue`

- [ ] **Step 1: Write the failing test**

```ts
// If a test harness does not exist, create a minimal unit-test setup first.
// Suggested test: emit models:changed and assert loadModels() is called.
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm -C frontend test` (or the repository’s existing test command)
Expected: FAIL (hooks not implemented).

- [ ] **Step 3: Write minimal implementation**

Implementation outline:
- Import `Events` from `@wailsio/runtime`.
- Add `let unsubscribeModelsChanged: (() => void) | null = null`.
- On mounted, subscribe:
  `unsubscribeModelsChanged = Events.On('models:changed', () => { void loadModels() })`.
- Add/extend `isTabActive` watcher to call `loadModels()` when the Knowledge tab becomes active.
- On unmount, call `unsubscribeModelsChanged?.()`.

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm -C frontend test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/knowledge/KnowledgePage.vue
# git commit -m "fix: refresh knowledge models on change"
```

## Chunk 2: Manual Verification

### Task 2: Verify UI behavior manually

**Files:**
- Modify: `frontend/src/pages/knowledge/KnowledgePage.vue`

- [ ] **Step 1: Run the app and open Knowledge page**

Run: `pnpm -C frontend dev`
Expected: App starts; Knowledge page opens.

- [ ] **Step 2: Validate ChatWiki model visibility**

Checklist:
- Bind or refresh ChatWiki models in Settings.
- Switch to Knowledge page personal tab.
- Open model selector and confirm ChatWiki models appear.
- Without switching pages, trigger model refresh; list updates after `models:changed`.

- [ ] **Step 3: Commit (if any fixes made during verification)**

```bash
git add frontend/src/pages/knowledge/KnowledgePage.vue
# git commit -m "chore: adjust model refresh behavior"
```
