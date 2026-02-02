package migrations

import (
	"context"
	"time"

	"willchat/internal/define"

	"github.com/uptrace/bun"
)

const dateTimeFormat = "2006-01-02 15:04:05"

// migrationProvider 迁移专用的供应商模型
type migrationProvider struct {
	bun.BaseModel `bun:"table:providers"`

	ID          int64  `bun:"id,pk,autoincrement"`
	ProviderID  string `bun:"provider_id,notnull"`
	Name        string `bun:"name,notnull"`
	Type        string `bun:"type,notnull"`
	Icon        string `bun:"icon,notnull"`
	IsBuiltin   bool   `bun:"is_builtin,notnull"`
	Enabled     bool   `bun:"enabled,notnull"`
	SortOrder   int    `bun:"sort_order,notnull"`
	APIEndpoint string `bun:"api_endpoint,notnull"`
	APIKey      string `bun:"api_key,notnull"`
	ExtraConfig string `bun:"extra_config,notnull"`
	CreatedAt   string `bun:"created_at,notnull"`
	UpdatedAt   string `bun:"updated_at,notnull"`
}

// migrationModel 迁移专用的模型模型
type migrationModel struct {
	bun.BaseModel `bun:"table:models"`

	ID         int64  `bun:"id,pk,autoincrement"`
	ProviderID string `bun:"provider_id,notnull"`
	ModelID    string `bun:"model_id,notnull"`
	Name       string `bun:"name,notnull"`
	Type       string `bun:"type,notnull"`
	IsBuiltin  bool   `bun:"is_builtin,notnull"`
	Enabled    bool   `bun:"enabled,notnull"`
	SortOrder  int    `bun:"sort_order,notnull"`
	CreatedAt  string `bun:"created_at,notnull"`
	UpdatedAt  string `bun:"updated_at,notnull"`
}

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// 创建供应商表
			createProviders := `
create table if not exists providers (
    id integer primary key autoincrement,
    created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp,
    
    provider_id varchar(64) not null unique,
    name varchar(64) not null,
    type varchar(16) not null default 'openai',
    icon text not null default '',
    is_builtin boolean not null default false,
    enabled boolean not null default false,
    sort_order integer not null default 0,
    
    api_endpoint varchar(1024) not null default '',
    api_key varchar(1024) not null default '',
    extra_config text not null default '{}'
);
`
			if _, err := db.ExecContext(ctx, createProviders); err != nil {
				return err
			}

			// 创建模型表
			createModels := `
create table if not exists models (
    id integer primary key autoincrement,
    created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp,
    
    provider_id varchar(64) not null,
    model_id varchar(128) not null,
    name varchar(128) not null,
    type varchar(16) not null default 'llm',
    is_builtin boolean not null default false,
    enabled boolean not null default true,
    sort_order integer not null default 0,
    
    unique(provider_id, model_id)
);
`
			if _, err := db.ExecContext(ctx, createModels); err != nil {
				return err
			}

			// 初始化内置供应商（使用 bun 批量插入，避免 SQL 注入风险）
			if len(define.BuiltinProviders) > 0 {
				now := time.Now().UTC().Format(dateTimeFormat)
				providers := make([]migrationProvider, 0, len(define.BuiltinProviders))
				for _, p := range define.BuiltinProviders {
					providers = append(providers, migrationProvider{
						ProviderID:  p.ProviderID,
						Name:        p.Name,
						Type:        p.Type,
						Icon:        p.Icon,
						IsBuiltin:   true,
						Enabled:     false,
						SortOrder:   p.SortOrder,
						APIEndpoint: p.APIEndpoint,
						APIKey:      "",
						ExtraConfig: "{}",
						CreatedAt:   now,
						UpdatedAt:   now,
					})
				}
				if _, err := db.NewInsert().Model(&providers).Exec(ctx); err != nil {
					return err
				}
			}

			// 初始化内置模型（使用 bun 批量插入，避免 SQL 注入风险）
			if len(define.BuiltinModels) > 0 {
				now := time.Now().UTC().Format(dateTimeFormat)
				models := make([]migrationModel, 0, len(define.BuiltinModels))
				for _, m := range define.BuiltinModels {
					models = append(models, migrationModel{
						ProviderID: m.ProviderID,
						ModelID:    m.ModelID,
						Name:       m.Name,
						Type:       m.Type,
						IsBuiltin:  true,
						Enabled:    true,
						SortOrder:  m.SortOrder,
						CreatedAt:  now,
						UpdatedAt:  now,
					})
				}
				if _, err := db.NewInsert().Model(&models).Exec(ctx); err != nil {
					return err
				}
			}

			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `drop table if exists models`); err != nil {
				return err
			}
			if _, err := db.ExecContext(ctx, `drop table if exists providers`); err != nil {
				return err
			}
			return nil
		},
	)
}
