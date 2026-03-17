# ChatWiki Binding Version Design

## Goal

When binding a ChatWiki account, accept a new `chatwiki_version` callback field, persist it in `chatwiki_bindings`, and default missing values to `dev`.

## Scope

- Persist `chatwiki_version` in the local binding record.
- Read `chatwiki_version` from the deep-link callback query.
- Return `chatwiki_version` from `GetBinding`.
- Keep existing token refresh behavior unchanged.

## Data Flow

1. `chatclaw://auth/callback` arrives with existing binding params plus optional `chatwiki_version`.
2. `internal/deeplink.HandleURL` parses the query and forwards `chatwiki_version` to `chatwiki.SaveBinding`.
3. `chatwiki.SaveBinding` writes the value into `chatwiki_bindings`.
4. `ChatWikiService.GetBinding` returns the stored value to the frontend.

## Storage

- New databases should create `chatwiki_bindings.chatwiki_version` as `TEXT NOT NULL DEFAULT 'dev'`.
- Existing databases should be migrated with `ALTER TABLE ... ADD COLUMN`.
- Application-level fallback should still coerce blank input to `dev` so callback omissions behave consistently.

## Risks

- Existing databases without the new column would fail inserts/selects unless migration is added.
- Blank callback values must not persist as empty strings, otherwise downstream code would see inconsistent semantics versus the DB default.

## Validation

- Regression test: saving a binding with `chatwiki_version=release` returns `release`.
- Regression test: saving a binding without `chatwiki_version` returns `dev`.
