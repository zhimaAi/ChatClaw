# Knowledge embedding model display name fix (ChatWiki)

## Summary
Fix the display label of ChatWiki embedding models in the Knowledge -> Embedding Settings dialog to match the AI assistant rule:
- show `model_supplier/uni_model_name` when both exist
- else `uni_model_name`
- else fallback to `model.name` then `model.model_id`

Scope: only affects display text in the embedding model dropdown; no change to selection keys or behavior.

## Current Behavior
Embedding model options show `model.name` only. For ChatWiki types, this can appear as numeric or incorrect labels.

## Proposed Change
Update `EmbeddingSettingsDialog.vue` to compute display name using ChatWiki-specific fields when present. No changes to data source, grouping, or selected key format.

## Components
- `frontend/src/pages/knowledge/components/EmbeddingSettingsDialog.vue`

## Data Flow
Provider model list -> render SelectItem label -> use new `getEmbeddingModelLabel()`.

## Error Handling
No changes.

## Testing
Manual:
- Open Knowledge -> Embedding Model Settings.
- Ensure ChatWiki models show `model_supplier/uni_model_name`.
- Verify models without those fields still show `name` (or `model_id` fallback).
