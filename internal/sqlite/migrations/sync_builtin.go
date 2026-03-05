package migrations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"chatclaw/internal/define"

	"github.com/uptrace/bun"
)

// SyncBuiltinProvidersAndModels synchronises the providers and models tables
// with the current BuiltinProviders / BuiltinModels slices in define/.
//
// Intended to be called from migration Up functions so that adding new models
// only requires updating builtin_providers.go and creating a one-liner
// migration.
//
// Behaviour:
//
//   - Providers: for each BuiltinProvider, if a row with the same provider_id
//     already exists, update name/type/icon/sort_order/api_endpoint (preserving
//     the user's api_key, enabled flag, and extra_config). If no row exists,
//     insert it.
//
//   - Models: for each BuiltinModel, if a row with the same (provider_id,
//     model_id) already exists — regardless of whether the user added it
//     manually or a prior migration inserted it — update name/type/sort_order
//     and mark is_builtin = true (preserving the user's enabled flag).
//     If no row exists, insert it as enabled.
//
//   - Stale models: builtin models present in the DB (is_builtin = true) but
//     no longer in BuiltinModels are marked is_builtin = false so they stay
//     visible but won't be re-managed.
func SyncBuiltinProvidersAndModels(ctx context.Context, db *bun.DB) error {
	now := time.Now().UTC().Format(dateTimeFormat)

	if err := syncProviders(ctx, db, now); err != nil {
		return fmt.Errorf("sync providers: %w", err)
	}
	if err := syncModels(ctx, db, now); err != nil {
		return fmt.Errorf("sync models: %w", err)
	}
	return nil
}

func syncProviders(ctx context.Context, db *bun.DB, now string) error {
	for _, p := range define.BuiltinProviders {
		var exists int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM providers WHERE provider_id = ?`, p.ProviderID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check provider %s: %w", p.ProviderID, err)
		}

		if exists > 0 {
			if _, err := db.ExecContext(ctx, `
				UPDATE providers
				SET name = ?, type = ?, icon = ?, sort_order = ?, api_endpoint = ?, updated_at = ?
				WHERE provider_id = ? AND is_builtin = 1
			`, p.Name, p.Type, p.Icon, p.SortOrder, p.APIEndpoint, now, p.ProviderID); err != nil {
				return fmt.Errorf("update provider %s: %w", p.ProviderID, err)
			}
		} else {
			enabled := p.ProviderID == "chatclaw"
			if _, err := db.ExecContext(ctx, `
				INSERT INTO providers
					(provider_id, name, type, icon, is_builtin, enabled, sort_order,
					 api_endpoint, api_key, extra_config, created_at, updated_at)
				VALUES (?, ?, ?, ?, 1, ?, ?, ?, '', '{}', ?, ?)
			`, p.ProviderID, p.Name, p.Type, p.Icon, enabled, p.SortOrder,
				p.APIEndpoint, now, now); err != nil {
				return fmt.Errorf("insert provider %s: %w", p.ProviderID, err)
			}
		}
	}
	return nil
}

func syncModels(ctx context.Context, db *bun.DB, now string) error {
	// Build a set of current builtin (provider_id, model_id) pairs for stale detection.
	type modelKey struct{ ProviderID, ModelID string }
	builtinSet := make(map[modelKey]struct{}, len(define.BuiltinModels))

	for _, m := range define.BuiltinModels {
		builtinSet[modelKey{m.ProviderID, m.ModelID}] = struct{}{}

		var exists int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM models WHERE provider_id = ? AND model_id = ?`,
			m.ProviderID, m.ModelID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check model %s/%s: %w", m.ProviderID, m.ModelID, err)
		}

		if exists > 0 {
			// Row exists (either from init migration, prior sync, or user manually added).
			// Update metadata and claim as builtin; preserve user's enabled flag.
			capabilities, _ := json.Marshal(m.Capabilities)
			if _, err := db.ExecContext(ctx, `
				UPDATE models
				SET name = ?, type = ?, sort_order = ?, capabilities = ?, is_builtin = 1, updated_at = ?
				WHERE provider_id = ? AND model_id = ?
			`, m.Name, m.Type, m.SortOrder, string(capabilities), now, m.ProviderID, m.ModelID); err != nil {
				return fmt.Errorf("update model %s/%s: %w", m.ProviderID, m.ModelID, err)
			}
		} else {
			capabilities, _ := json.Marshal(m.Capabilities)
			if _, err := db.ExecContext(ctx, `
				INSERT INTO models
					(provider_id, model_id, name, type, capabilities, is_builtin, enabled, sort_order, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, 1, 1, ?, ?, ?)
			`, m.ProviderID, m.ModelID, m.Name, m.Type, string(capabilities), m.SortOrder, now, now); err != nil {
				return fmt.Errorf("insert model %s/%s: %w", m.ProviderID, m.ModelID, err)
			}
		}
	}

	// Mark stale builtin models: rows that are is_builtin = true in the DB but
	// no longer in BuiltinModels. We flip is_builtin to false so they remain
	// usable but won't be managed in future syncs.
	rows, err := db.QueryContext(ctx,
		`SELECT provider_id, model_id FROM models WHERE is_builtin = 1`)
	if err != nil {
		return fmt.Errorf("query builtin models: %w", err)
	}
	defer rows.Close()

	var staleKeys []modelKey
	for rows.Next() {
		var k modelKey
		if err := rows.Scan(&k.ProviderID, &k.ModelID); err != nil {
			return fmt.Errorf("scan builtin model: %w", err)
		}
		if _, ok := builtinSet[k]; !ok {
			staleKeys = append(staleKeys, k)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate builtin models: %w", err)
	}

	for _, k := range staleKeys {
		if _, err := db.ExecContext(ctx, `
			UPDATE models SET is_builtin = 0, updated_at = ?
			WHERE provider_id = ? AND model_id = ?
		`, now, k.ProviderID, k.ModelID); err != nil {
			return fmt.Errorf("unmark stale model %s/%s: %w", k.ProviderID, k.ModelID, err)
		}
	}

	return nil
}
