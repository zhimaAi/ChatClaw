# ChatWiki Dev Model Gating Design

## Goal

When the provider is ChatWiki and the current binding has `chatwiki_version = dev`, the model list should be treated as unavailable, and the provider label should show `（非ChatWiki Cloud）`.

## Scope

- Treat ChatWiki `dev` bindings the same as "not selectable" in model pickers.
- Apply the rule consistently to assistant, agent settings, and knowledge-related model selectors that already reuse `chatwikiModelAvailability.ts`.
- Update provider label formatting for the ChatWiki provider when the binding is `dev`.

## Design

1. Centralize ChatWiki model availability in `frontend/src/lib/chatwikiModelAvailability.ts`.
2. Replace the current boolean-only binding checks with a status derived from the binding payload:
   - no binding: unavailable
   - `chatwiki_version = dev`: unavailable
   - other bound versions: available
3. Keep model labels unchanged.
4. Update provider labels:
   - unbound ChatWiki: `（未登录）`
   - bound ChatWiki with `dev`: `（非ChatWiki Cloud）`
   - other cases: unchanged

## Risks

- Some pages currently use `Boolean(binding)` directly, so partial updates would produce inconsistent behavior.
- Provider label formatting must avoid stacking both suffixes at once.

## Validation

- Unit test: ChatWiki `dev` binding disables model selection.
- Unit test: fallback model selection skips ChatWiki `dev`.
- Unit test: provider label shows `（非ChatWiki Cloud）` for ChatWiki `dev`.
- Focused frontend test pass for `chatwikiModelAvailability`.
