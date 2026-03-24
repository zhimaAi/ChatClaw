package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatclaw/internal/define"

	"github.com/uptrace/bun"
)

type optionalModelColumn struct {
	Name  string
	Value func(model define.BuiltinModelConfig) (any, error)
}

var modelOptionalColumns = []optionalModelColumn{
	{
		Name: "capabilities",
		Value: func(model define.BuiltinModelConfig) (any, error) {
			capabilities, err := json.Marshal(model.Capabilities)
			if err != nil {
				return nil, err
			}
			return string(capabilities), nil
		},
	},
}

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
			if _, err := db.ExecContext(ctx, `
				INSERT INTO providers
					(provider_id, name, type, icon, is_builtin, enabled, sort_order,
					 api_endpoint, api_key, extra_config, created_at, updated_at)
				VALUES (?, ?, ?, ?, 1, ?, ?, ?, '', '{}', ?, ?)
			`, p.ProviderID, p.Name, p.Type, p.Icon, false, p.SortOrder,
				p.APIEndpoint, now, now); err != nil {
				return fmt.Errorf("insert provider %s: %w", p.ProviderID, err)
			}
		}
	}
	return nil
}

func syncModels(ctx context.Context, db *bun.DB, now string) error {
	availableOptionalColumns, err := availableModelOptionalColumns(ctx, db)
	if err != nil {
		return fmt.Errorf("resolve optional model columns: %w", err)
	}

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
			updateSQL, updateArgs, err := buildModelUpdateSQL(m, now, availableOptionalColumns)
			if err != nil {
				return fmt.Errorf("build update model %s/%s: %w", m.ProviderID, m.ModelID, err)
			}
			if _, err := db.ExecContext(ctx, updateSQL, updateArgs...); err != nil {
				return fmt.Errorf("update model %s/%s: %w", m.ProviderID, m.ModelID, err)
			}
		} else {
			insertSQL, insertArgs, err := buildModelInsertSQL(m, now, availableOptionalColumns)
			if err != nil {
				return fmt.Errorf("build insert model %s/%s: %w", m.ProviderID, m.ModelID, err)
			}
			if _, err := db.ExecContext(ctx, insertSQL, insertArgs...); err != nil {
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

func availableModelOptionalColumns(ctx context.Context, db *bun.DB) ([]optionalModelColumn, error) {
	available := make([]optionalModelColumn, 0, len(modelOptionalColumns))
	for _, column := range modelOptionalColumns {
		exists, err := hasColumn(ctx, db, "models", column.Name)
		if err != nil {
			return nil, fmt.Errorf("check models.%s: %w", column.Name, err)
		}
		if exists {
			available = append(available, column)
		}
	}
	return available, nil
}

func buildModelUpdateSQL(model define.BuiltinModelConfig, now string, optionalColumns []optionalModelColumn) (string, []any, error) {
	assignments := []string{"name = ?", "type = ?", "sort_order = ?"}
	args := []any{model.Name, model.Type, model.SortOrder}

	for _, column := range optionalColumns {
		value, err := column.Value(model)
		if err != nil {
			return "", nil, fmt.Errorf("resolve %s: %w", column.Name, err)
		}
		assignments = append(assignments, fmt.Sprintf("%s = ?", column.Name))
		args = append(args, value)
	}

	assignments = append(assignments, "is_builtin = 1", "updated_at = ?")
	args = append(args, now, model.ProviderID, model.ModelID)

	sql := fmt.Sprintf(
		"UPDATE models SET %s WHERE provider_id = ? AND model_id = ?",
		strings.Join(assignments, ", "),
	)
	return sql, args, nil
}

func buildModelInsertSQL(model define.BuiltinModelConfig, now string, optionalColumns []optionalModelColumn) (string, []any, error) {
	columns := []string{"provider_id", "model_id", "name", "type"}
	args := []any{model.ProviderID, model.ModelID, model.Name, model.Type}

	for _, column := range optionalColumns {
		value, err := column.Value(model)
		if err != nil {
			return "", nil, fmt.Errorf("resolve %s: %w", column.Name, err)
		}
		columns = append(columns, column.Name)
		args = append(args, value)
	}

	columns = append(columns, "is_builtin", "enabled", "sort_order", "created_at", "updated_at")
	args = append(args, 1, 1, model.SortOrder, now, now)

	sql := fmt.Sprintf(
		"INSERT INTO models (%s) VALUES (%s)",
		strings.Join(columns, ", "),
		strings.Join(questionMarks(len(columns)), ", "),
	)
	return sql, args, nil
}

func questionMarks(n int) []string {
	placeholders := make([]string, n)
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return placeholders
}

func hasColumn(ctx context.Context, db *bun.DB, tableName, columnName string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			defaultV  sql.NullString
			primaryID int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &primaryID); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}

	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}
