package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
-- 为 agents 表添加 library_ids 字段（存储关联知识库ID的JSON数组）
ALTER TABLE agents ADD COLUMN library_ids TEXT NOT NULL DEFAULT '[]';

-- 为 conversations 表添加模型覆盖字段
ALTER TABLE conversations ADD COLUMN llm_provider_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN llm_model_id VARCHAR(128) NOT NULL DEFAULT '';

-- 消息表 - 用于存储 ReAct 智能体循环的聊天消息（兼容 Eino ADK）
CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

	conversation_id INTEGER NOT NULL,

	-- 消息角色: user / assistant / system / tool
	role VARCHAR(16) NOT NULL,
	
	-- 消息内容（assistant: 最终回复; tool: 工具返回的JSON结果）
	content TEXT NOT NULL DEFAULT '',
	
	-- 模型信息（生成该回复的模型，仅 assistant 消息有值）
	provider_id VARCHAR(64) NOT NULL DEFAULT '',
	model_id VARCHAR(128) NOT NULL DEFAULT '',

	-- 消息状态: pending / streaming / success / error / cancelled
	status VARCHAR(16) NOT NULL DEFAULT 'success',
	error TEXT NOT NULL DEFAULT '',

	-- Token 用量统计（仅 assistant 消息）
	input_tokens INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,

	-- 模型响应元信息
	finish_reason VARCHAR(16) NOT NULL DEFAULT '',  -- stop / tool_calls / length / content_filter

	-- ReAct / 工具调用支持（兼容 Eino ADK）
	-- assistant 消息: 工具调用的JSON数组
	-- 示例: [{"ID":"call_xxx","Type":"function","Function":{"Name":"search","Arguments":"{...}"}}]
	tool_calls TEXT NOT NULL DEFAULT '[]',
	
	-- tool 消息: 该消息响应的工具调用ID和名称
	tool_call_id VARCHAR(64) NOT NULL DEFAULT '',
	tool_call_name VARCHAR(64) NOT NULL DEFAULT '',  -- 工具名称，便于查询
	
	-- 扩展内容（可选，部分模型支持）
	thinking_content TEXT NOT NULL DEFAULT '',  -- 思考链/推理内容（如 Claude thinking）

	FOREIGN KEY(conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id, created_at);

-- 消息附件表（图片、文件等多模态输入）
CREATE TABLE IF NOT EXISTS message_attachments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

	message_id INTEGER NOT NULL,
	type VARCHAR(16) NOT NULL,               -- 类型: image / file / audio / video
	name TEXT NOT NULL,                      -- 原始文件名
	path TEXT NOT NULL,                      -- 本地存储路径
	mime_type VARCHAR(128) NOT NULL DEFAULT '',
	size INTEGER NOT NULL DEFAULT 0,         -- 文件大小（字节）
	width INTEGER NOT NULL DEFAULT 0,        -- 图片宽度
	height INTEGER NOT NULL DEFAULT 0,       -- 图片高度

	FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_message_attachments_message_id ON message_attachments(message_id);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP INDEX IF EXISTS idx_message_attachments_message_id;
DROP TABLE IF EXISTS message_attachments;

DROP INDEX IF EXISTS idx_messages_conversation_id;
DROP TABLE IF EXISTS messages;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
