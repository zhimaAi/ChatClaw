package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
-- 托盘相关设置
('show_tray_icon', 'true', 'boolean', 'tools', '托盘：是否显示系统托盘图标', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('minimize_to_tray_on_close', 'true', 'boolean', 'tools', '托盘：关闭窗口时是否最小化到系统托盘', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),

-- 悬浮窗相关设置
('show_floating_window', 'true', 'boolean', 'tools', '悬浮窗：是否显示桌面悬浮窗', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),

-- 划词搜索相关设置
('enable_selection_search', 'true', 'boolean', 'tools', '划词搜索：是否启用划词搜索功能', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// 回滚时仅删除本次 migration 写入的 keys，避免误删整张 settings 表
			if _, err := db.ExecContext(ctx, `
DELETE FROM settings WHERE key IN (
  'show_tray_icon',
  'minimize_to_tray_on_close',
  'show_floating_window',
  'enable_selection_search'
);
`); err != nil {
				return err
			}
			return nil
		},
	)
}
