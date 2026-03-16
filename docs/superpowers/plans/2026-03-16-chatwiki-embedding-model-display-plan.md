# ChatWiki Embedding Model Display Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align ChatWiki embedding model display labels in Knowledge -> Embedding Settings with the AI assistant rule (`model_supplier/uni_model_name`).

**Architecture:** Add a small, local display-name helper in the embedding settings dialog that formats ChatWiki models using `model_supplier`/`uni_model_name`, and falls back to `name`/`model_id` for other providers. Wire both the selected label and dropdown items to use this helper.

**Tech Stack:** Vue 3 + TypeScript, Wails bindings, Reka UI Select

---

## File Structure
- Modify: `frontend/src/pages/knowledge/components/EmbeddingSettingsDialog.vue`
  - Responsibility: UI for embedding model selection and dimension settings in Knowledge page.
  - Change: add display-name helper and use it for the Select value label and list items.

## Chunk 1: Display Name Helper + Wiring

### Task 1: Add ChatWiki-aware display label helper

**Files:**
- Modify: `frontend/src/pages/knowledge/components/EmbeddingSettingsDialog.vue`

- [ ] **Step 1: Write the failing test**

```ts
// If a test harness does not exist, create a minimal unit-test setup first.
// Suggested test: helper returns "supplier/uni" for ChatWiki models.
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm -C frontend test` (or the repository’s existing test command)
Expected: FAIL (helper not implemented).

- [ ] **Step 3: Write minimal implementation**

Implementation outline:
- Add a local `type ChatwikiDisplayModel = Model & { model_supplier?: string; uni_model_name?: string }`.
- Add `normalizeText()` and `getEmbeddingModelLabel(providerId: string, model: Model)` helpers.
- Logic:
  - If `providerId === 'chatwiki'` and both `model_supplier` and `uni_model_name` present, return `${supplier}/${uni}`.
  - Else if `uni_model_name` present, return it.
  - Else return `model.name || model.model_id || '-'`.
- Update:
  - `embeddingCurrentLabel` to use `getEmbeddingModelLabel(pid, model)`.
  - `<SelectItem>` label to render `getEmbeddingModelLabel(g.provider.provider_id, m)`.

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm -C frontend test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/knowledge/components/EmbeddingSettingsDialog.vue
# plus any test files if added
# git commit -m "fix: align ChatWiki embedding model display"
```

## Chunk 2: Manual Verification

### Task 2: Verify UI behavior manually

**Files:**
- Modify: `frontend/src/pages/knowledge/components/EmbeddingSettingsDialog.vue`

- [ ] **Step 1: Run the app and open Embedding Settings**

Run: `pnpm -C frontend dev`
Expected: App starts; Embedding Settings dialog opens.

- [ ] **Step 2: Validate display labels**

Checklist:
- ChatWiki models show `model_supplier/uni_model_name` when both exist.
- If only `uni_model_name` exists, show it.
- If neither exists, show `model.name` or `model.model_id`.
- Non-ChatWiki providers unchanged.

- [ ] **Step 3: Commit (if any fixes made during verification)**

```bash
git add frontend/src/pages/knowledge/components/EmbeddingSettingsDialog.vue
# git commit -m "chore: adjust embedding model display labels"
```
