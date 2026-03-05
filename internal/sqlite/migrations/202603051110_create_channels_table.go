package migrations

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// migrationChannel 迁移专用的频道/机器人模型
type migrationChannel struct {
	bun.BaseModel `bun:"table:channels"`

	ID             int64  `bun:"id,pk,autoincrement"`
	Platform       string `bun:"platform,notnull"`        // feishu, tg, wechat
	Name           string `bun:"name,notnull"`            // 机器人名称
	Avatar         string `bun:"avatar,notnull"`          // 机器人头像
	Enabled        bool   `bun:"enabled,notnull"`         // 是否启用连接
	ConnectionType string `bun:"connection_type,notnull"` // gateway (长连), webhook (HTTP)

	// 核心配置：存储 AppID, Secret, Token 等 JSON 字符串
	ExtraConfig string `bun:"extra_config,notnull"`

	Status          string     `bun:"status,notnull"`    // online, offline, error
	LastConnectedAt *time.Time `bun:"last_connected_at"` // 上次连接时间
	CreatedAt       string     `bun:"created_at,notnull"`
	UpdatedAt       string     `bun:"updated_at,notnull"`
}

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// 创建频道表
			createChannels := `
create table if not exists channels (
    id integer primary key autoincrement,
    created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp,
    
    platform varchar(32) not null,
    name varchar(64) not null,
    avatar text not null default '',
    enabled boolean not null default false,
    connection_type varchar(16) not null default 'gateway',
    
    extra_config text not null default '{}',
    
    status varchar(16) not null default 'offline',
    last_connected_at datetime
);
`
			if _, err := db.ExecContext(ctx, createChannels); err != nil {
				return err
			}

			// 为 platform 增加索引，方便 Gateway Manager 快速筛选
			if _, err := db.ExecContext(ctx, `create index if not exists idx_channels_platform on channels(platform)`); err != nil {
				return err
			}

			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `drop table if exists channels`); err != nil {
				return err
			}
			return nil
		},
	)
}
