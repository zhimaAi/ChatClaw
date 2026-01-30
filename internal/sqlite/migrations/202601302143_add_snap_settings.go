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
('show_ai_send_button', 'true', 'boolean', 'snap', 'AI回复: 是否在界面显示“发送到聊天”按钮', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('send_key_strategy', 'enter', 'string', 'snap', '发送消息的快捷键模式 (如: enter, ctrl_enter)', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('show_ai_edit_button', 'true', 'boolean', 'snap', 'AI回复: 是否在界面显示“编辑内容”按钮', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_wechat', 'true', 'boolean', 'snap', '吸附应用：微信', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_wecom', 'true', 'boolean', 'snap', '吸附应用: 企业微信', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_qq', 'true', 'boolean', 'snap', '吸附应用: QQ', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_dingtalk', 'true', 'boolean', 'snap', '吸附应用: 钉钉', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_feishu', 'true', 'boolean', 'snap', '吸附应用: 飞书', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_douyin', 'true', 'boolean', 'snap', '吸附应用: 抖音', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS settings;`); err != nil {
				return err
			}
			return nil
		},
	)
}
