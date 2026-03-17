# ChatWiki Model Persistence Design

## Goal

Make ChatWiki models persist in the local `models` table instead of only existing in memory, remove assistant frontend display compatibility code that depends on ChatWiki-only fields, and make ChatWiki provider requests use the OpenAI-compatible `/chatclaw/v1/chat/completions` endpoint.

## Scope

- Sync ChatWiki remote model catalog into the local `models` table.
- Trigger sync whenever `/manage/chatclaw/showModelConfigList` is fetched.
- Store ChatWiki models with:
  - `provider_id = "chatwiki"`
  - `model_id = uni_model_name`
- Treat ChatWiki models as ordinary provider models in backend validation and frontend display.
- Switch ChatWiki chat completion requests to the OpenAI-compatible endpoint.

## Constraints

- `models` table has `unique(provider_id, model_id)`.
- For ChatWiki, `chatwiki + uni_model_name` is guaranteed unique by the remote API.
- Frontend compatibility logic for `model_supplier/uni_model_name` should be removed on the current branch.
- Existing unrelated workspace changes must be preserved.

## Current State

- `internal/services/chatwiki/model_catalog.go` fetches and caches ChatWiki model catalog in memory.
- `internal/services/providers/service.go` special-cases `chatwiki` in `GetProviderWithModels` and builds model groups from the in-memory catalog instead of the `models` table.
- `internal/services/agents/service.go` special-cases `chatwiki` in `ensureLLMModelExists`, validating against the in-memory catalog.
- Assistant frontend files format ChatWiki labels as `model_supplier/uni_model_name`, which is only needed because ChatWiki models bypass the normal `models` table path.
- `internal/services/chatwiki/service.go` team chat still calls `/manage/chatclaw/chat/completions`.

## Design

### 1. Persist ChatWiki models to `models`

Add a sync path in the ChatWiki catalog fetch flow:

- After `showModelConfigList` is decoded, map catalog items into local `models` records.
- Use `uni_model_name` as the persisted `model_id`.
- Use `uni_model_name` as `name`, so normal frontend display works without special formatting.
- Map remote types into local `type` values: `llm`, `embedding`, `rerank`.
- Upsert existing rows by `provider_id + model_id`.
- Delete local ChatWiki rows that no longer exist remotely.

This keeps the local table authoritative for downstream reads while preserving the existing in-memory catalog for fields still needed by runtime resolution, such as `self_owned_model_config_id`.

### 2. Move reads back to the shared provider/model path

After sync is in place:

- Remove ChatWiki special handling from `ProvidersService.GetProviderWithModels`.
- Remove ChatWiki special handling from `ensureLLMModelExists`.
- Let all model reads and validations use the local `models` table.

This makes ChatWiki behave like any other provider from the app's point of view.

### 3. Remove assistant frontend compatibility display code

Delete ChatWiki-specific label formatting from:

- `frontend/src/pages/assistant/composables/useModelSelection.ts`
- `frontend/src/pages/assistant/components/ChatInputArea.vue`
- `frontend/src/pages/assistant/components/AgentSettingsDialog.vue`

After persistence, those views should display `model.name` or `model.model_id` only.

### 4. Switch ChatWiki chat completions to OpenAI compatibility

Update ChatWiki chat request code to call:

- `/chatclaw/v1/chat/completions`

Use the OpenAI-compatible base URL already derived from the ChatWiki binding where possible. Keep existing auth headers and SSE handling unless the endpoint requires a change.

## Testing Strategy

- Add backend tests that prove ChatWiki catalog fetch persists models into `models`.
- Add backend tests that prove stale ChatWiki rows are removed on re-sync.
- Add backend tests that prove `ensureLLMModelExists` accepts persisted ChatWiki models from the database.
- Add backend tests that prove ChatWiki team chat targets `/chatclaw/v1/chat/completions`.
- Run focused Go tests for changed packages.
- Run targeted frontend type/build verification for changed assistant files if available.

## Risks

- Persisting `name = uni_model_name` intentionally drops supplier-prefixed display text. This matches the requested removal of frontend compatibility logic.
- The in-memory catalog must remain usable for runtime fields not stored in `models`, especially `self_owned_model_config_id`.
- Team chat SSE parsing assumes the new OpenAI-compatible route still returns the current event stream format; if not, request/response adaptation may be needed in the same change.
