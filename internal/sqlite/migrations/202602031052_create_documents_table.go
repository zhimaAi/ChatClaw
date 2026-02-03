package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
create table if not exists documents (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,

	library_id integer not null,
	original_name text not null,
	thumb_icon text,
	file_size integer not null default 0,
	content_hash text not null,
	
	extension text not null,
	mime_type text not null,
	source_type text not null,
	
	local_path text,
	web_url text,

	parsing_status integer not null default 0,  -- 0=pending, 1=processing, 2=completed, 3=failed
	parsing_progress integer not null default 0,
	parsing_error text not null default '',

	word_total integer not null default 0,
	split_total integer not null default 0
);
CREATE INDEX idx_docs_library_id ON documents(library_id);
CREATE UNIQUE INDEX idx_docs_library_hash ON documents(library_id, content_hash);

CREATE VIRTUAL TABLE doc_fts USING fts5(
    content,
    content='document_nodes',
    content_rowid='id',
    tokenize='jieba'
);

CREATE VIRTUAL TABLE doc_vec USING vec0(
	id INTEGER PRIMARY KEY,
    content FLOAT[1536]  -- 会根据全局配置的向量重建表,维度是动态的
);

CREATE TABLE IF NOT EXISTS document_nodes (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,
	
	library_id integer not null,
	document_id integer not null,
	
	content text not null,  -- 可能是原始块，也可能是 AI 生成的摘要
	level integer not null default 0,  -- 0: 原始块, 1: 一级摘要, 2: 总括摘要
	parent_id integer,  -- RAPTOR 向上追溯
	chunk_order integer not null default 0  -- 同一层级内的顺序
);
CREATE INDEX idx_nodes_library_id ON document_nodes(library_id);
CREATE INDEX idx_nodes_document_id ON document_nodes(document_id);
CREATE INDEX idx_nodes_parent_id ON document_nodes(parent_id);
CREATE INDEX idx_nodes_level ON document_nodes(level);
CREATE INDEX idx_nodes_doc_level_order ON document_nodes(document_id, level, chunk_order);

-- 当 document_nodes 插入新行时，同步更新索引
CREATE TRIGGER doc_nodes_ai AFTER INSERT ON document_nodes BEGIN
  INSERT INTO doc_fts(rowid, content) VALUES (new.id, new.content);
END;

-- 当 document_nodes 删除行时，同步删除索引
CREATE TRIGGER doc_nodes_ad AFTER DELETE ON document_nodes BEGIN
  INSERT INTO doc_fts(doc_fts, rowid, content) VALUES('delete', old.id, old.content);
END;

-- 当内容修改时，更新索引
CREATE TRIGGER doc_nodes_au AFTER UPDATE ON document_nodes BEGIN
  INSERT INTO doc_fts(doc_fts, rowid, content) VALUES('delete', old.id, old.content);
  INSERT INTO doc_fts(rowid, content) VALUES (new.id, new.content);
END;
`

			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP TRIGGER IF EXISTS doc_nodes_au;
DROP TRIGGER IF EXISTS doc_nodes_ad;
DROP TRIGGER IF EXISTS doc_nodes_ai;
DROP TABLE IF EXISTS document_nodes;
DROP TABLE IF EXISTS doc_vec;
DROP TABLE IF EXISTS doc_fts;
DROP TABLE IF EXISTS documents;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
