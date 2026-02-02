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
('snap_wechat', 'false', 'boolean', 'snap', '吸附应用：微信', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_wecom', 'false', 'boolean', 'snap', '吸附应用: 企业微信', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_qq', 'false', 'boolean', 'snap', '吸附应用: QQ', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_dingtalk', 'false', 'boolean', 'snap', '吸附应用: 钉钉', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_feishu', 'false', 'boolean', 'snap', '吸附应用: 飞书', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('snap_douyin', 'false', 'boolean', 'snap', '吸附应用: 抖音', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
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
  'show_ai_send_button',
  'send_key_strategy',
  'show_ai_edit_button',
  'snap_wechat',
  'snap_wecom',
  'snap_qq',
  'snap_dingtalk',
  'snap_feishu',
  'snap_douyin'
);
`); err != nil {
				return err
			}
			return nil
		},
	)
}
