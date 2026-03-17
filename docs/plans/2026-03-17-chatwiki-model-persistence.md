# ChatWiki Model Persistence Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Persist ChatWiki models into the local `models` table, remove assistant frontend ChatWiki display compatibility code, and route ChatWiki chat completions through the OpenAI-compatible endpoint.

**Architecture:** Keep ChatWiki remote catalog fetching as the source of truth for remote metadata, but synchronize its model list into the shared local `models` table each time `/manage/chatclaw/showModelConfigList` is fetched. Then remove backend and frontend ChatWiki model-list special cases so provider/model reads use the same path as other providers.

**Tech Stack:** Go, Bun, SQLite, Vue 3, TypeScript, Wails

---

### Task 1: Add failing backend tests for ChatWiki model persistence

**Files:**
- Modify: `internal/services/chatwiki/openai_compat_test.go`
- Modify: `internal/services/agents/service.go`
- Test: `internal/services/chatwiki/openai_compat_test.go`

**Step 1: Write the failing test**

- Add a test that fetches `showModelConfigList` data containing ChatWiki LLM and embedding models with `uni_model_name`.
- Assert the fetch path synchronizes rows into `models` with:
  - `provider_id = "chatwiki"`
  - `model_id = uni_model_name`
  - `name = uni_model_name`

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/chatwiki -run Test.*ChatWiki.*Model.* -count=1`

Expected: FAIL because catalog fetch does not yet persist rows.

**Step 3: Write minimal implementation**

- Add sync helpers in `internal/services/chatwiki` that map decoded catalog items into local `models` rows.
- Call sync from the `showModelConfigList` fetch path after decoding succeeds.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/chatwiki -run Test.*ChatWiki.*Model.* -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/services/chatwiki/openai_compat_test.go internal/services/chatwiki/*.go
git commit -m "feat: persist chatwiki catalog models"
```

### Task 2: Add failing backend tests for ChatWiki stale-model cleanup and DB-based validation

**Files:**
- Modify: `internal/services/chatwiki/openai_compat_test.go`
- Add: `internal/services/agents/service_test.go`
- Modify: `internal/services/agents/service.go`

**Step 1: Write the failing test**

- Add a re-sync test that:
  - syncs an initial ChatWiki model set,
  - syncs a second set with one removed model,
  - asserts the removed row no longer exists in `models`.
- Add a validation test for `ensureLLMModelExists` proving `chatwiki` now succeeds from the database without using in-memory catalog special handling.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/chatwiki ./internal/services/agents -run "Test.*ChatWiki.*|TestEnsureLLMModelExists.*" -count=1`

Expected: FAIL because stale cleanup and DB-only validation are not both implemented yet.

**Step 3: Write minimal implementation**

- Reuse the provider sync pattern: compare remote and local rows, delete stale rows, upsert changed rows.
- Remove the `providerID == "chatwiki"` branch in `ensureLLMModelExists` so it always validates against `models`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/chatwiki ./internal/services/agents -run "Test.*ChatWiki.*|TestEnsureLLMModelExists.*" -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/services/chatwiki/*.go internal/services/agents/service.go internal/services/agents/service_test.go
git commit -m "refactor: validate chatwiki models from db"
```

### Task 3: Add failing backend test for provider model loading and OpenAI chat completions path

**Files:**
- Modify: `internal/services/providers/service.go`
- Modify: `internal/services/chatwiki/service.go`
- Add or modify tests in:
  - `internal/services/providers/*_test.go`
  - `internal/services/chatwiki/*_test.go`

**Step 1: Write the failing test**

- Add a provider-service test showing `GetProviderWithModels("chatwiki")` reads synchronized local `models` rows instead of constructing groups from the in-memory catalog.
- Add a ChatWiki service test showing the team chat request path is `/chatclaw/v1/chat/completions`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/providers ./internal/services/chatwiki -run "TestGetProviderWithModelsChatWiki.*|Test.*ChatCompletions.*" -count=1`

Expected: FAIL because the provider service still special-cases ChatWiki and team chat still uses the old route.

**Step 3: Write minimal implementation**

- Remove the `chatwiki` special branch from `GetProviderWithModels`.
- Change ChatWiki team chat request candidates to use the OpenAI-compatible endpoint.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/providers ./internal/services/chatwiki -run "TestGetProviderWithModelsChatWiki.*|Test.*ChatCompletions.*" -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/services/providers/service.go internal/services/chatwiki/service.go internal/services/providers/*_test.go internal/services/chatwiki/*_test.go
git commit -m "refactor: use shared model loading for chatwiki"
```

### Task 4: Add failing frontend checks for assistant model display simplification

**Files:**
- Modify: `frontend/src/pages/assistant/composables/useModelSelection.ts`
- Modify: `frontend/src/pages/assistant/components/ChatInputArea.vue`
- Modify: `frontend/src/pages/assistant/components/AgentSettingsDialog.vue`

**Step 1: Write the failing test**

- If assistant model-selection tests already exist, extend them to assert ChatWiki models display `name`/`model_id` without supplier formatting.
- If no existing test harness covers these files, use TypeScript-level changes as the check: remove `model_supplier`/`uni_model_name` typing and ensure no remaining references compile.

**Step 2: Run test to verify it fails**

Run: `pnpm exec vue-tsc --noEmit`

Expected: FAIL after removing helper usages but before all references are cleaned up.

**Step 3: Write minimal implementation**

- Delete `ChatwikiDisplayModel`, `normalizeText` branches, and `providerId === "chatwiki"` display formatting from the three assistant files.
- Render labels from `model.name || model.model_id || "-"`.

**Step 4: Run test to verify it passes**

Run: `pnpm exec vue-tsc --noEmit`

Expected: PASS.

**Step 5: Commit**

```bash
git add frontend/src/pages/assistant/composables/useModelSelection.ts frontend/src/pages/assistant/components/ChatInputArea.vue frontend/src/pages/assistant/components/AgentSettingsDialog.vue
git commit -m "refactor: remove chatwiki model display compatibility"
```

### Task 5: Full verification

**Files:**
- Verify touched backend and frontend files from previous tasks

**Step 1: Run focused backend verification**

Run: `go test ./internal/services/chatwiki ./internal/services/providers ./internal/services/agents -count=1`

Expected: PASS.

**Step 2: Run focused frontend verification**

Run: `pnpm exec vue-tsc --noEmit`

Expected: PASS.

**Step 3: Re-check requirements**

- ChatWiki models sync into `models` whenever `showModelConfigList` is fetched.
- `provider_id = "chatwiki"` and `model_id = uni_model_name`.
- Assistant frontend no longer contains ChatWiki-only display compatibility code on the current branch.
- ChatWiki service uses `/chatclaw/v1/chat/completions`.

**Step 4: Commit**

```bash
git status --short
git commit -m "feat: persist chatwiki models locally"
```
