# Knowledge page ChatWiki model list refresh

## Summary
Fix missing ChatWiki models in the Knowledge page (personal tab) model selector by refreshing the model list when models change and when the Knowledge tab becomes active.

Scope: only the Knowledge page model list refresh behavior. No changes to model display rules or provider enable semantics.

## Current Behavior
The Knowledge page loads models on initial mount only. If ChatWiki models are bound or refreshed later, the Knowledge page does not update, so the model selector lacks ChatWiki models even though they appear on the Assistant page.

## Proposed Change
Add model list refresh hooks in `KnowledgePage.vue`:
- Subscribe to `models:changed` (same event as Assistant page) and call `loadModels()`.
- When the Knowledge tab becomes active, call `loadModels()` to ensure stale lists are refreshed.
- Unsubscribe on unmount.

## Components
- `frontend/src/pages/knowledge/KnowledgePage.vue`

## Data Flow
Settings/ModelService emits `models:changed` -> Knowledge page loads models -> ChatInputArea receives `providersWithModels` -> model selector shows ChatWiki models.

## Error Handling
Reuse existing `loadModels()` handling (toasts on partial/full failure). No new error flows.

## Testing
Manual:
- Bind or refresh ChatWiki models in Settings.
- Switch to Knowledge page (personal tab) and open model selector.
- Verify ChatWiki models appear.
- Without switching pages, trigger model refresh and verify list updates after `models:changed`.
