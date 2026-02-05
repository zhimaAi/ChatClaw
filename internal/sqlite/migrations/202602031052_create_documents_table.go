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
	name_tokens text not null default '', -- 预分词后的文件名 token 文本（用于 FTS）
	thumb_icon text,
	file_size integer not null default 0,
	content_hash text not null,
	
	extension text not null,
	mime_type text not null,
	source_type text not null, -- local,web
	
	local_path text,
	web_url text,

	processing_run_id text not null default '', -- 当前解析/向量化流水线的运行ID（用于重跑、防止旧任务回写）

	parsing_status integer not null default 0,  -- 0=pending, 1=processing, 2=completed, 3=failed
	parsing_progress integer not null default 0,
	parsing_error text not null default '',

	embedding_status integer not null default 0,  -- 0=pending, 1=processing, 2=completed, 3=failed
	embedding_progress integer not null default 0,
	embedding_error text not null default '',

	word_total integer not null default 0,
	split_total integer not null default 0,

	foreign key(library_id) references library(id) on delete cascade
);
CREATE INDEX idx_docs_library_id ON documents(library_id);
CREATE UNIQUE INDEX idx_docs_library_hash ON documents(library_id, content_hash);

CREATE VIRTUAL TABLE doc_fts USING fts5(
    -- 预分词后的 token 文本（由 Go 写入；用空格分隔）
    tokens,
	-- 用于过滤的元信息（不参与倒排索引）
	library_id UNINDEXED,
	document_id UNINDEXED,
	level UNINDEXED,
    -- contentless FTS: 只存索引，不保存内容副本；查询用 rowid 回表 document_nodes 拿 content/元信息
    content='',
    tokenize='unicode61'
);

-- documents 文件名全文检索（contentless FTS）
CREATE VIRTUAL TABLE doc_name_fts USING fts5(
    -- 预分词后的 token 文本（由 Go 写入；用空格分隔）
    name_tokens,
	-- 用于过滤的元信息（不参与倒排索引）
	library_id UNINDEXED,
	document_id UNINDEXED,
    content='',
    tokenize='unicode61'
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
	content_tokens text not null default '',  -- 预分词后的 token 文本（用于 FTS）
	level integer not null default 0,  -- 0: 原始块, 1: 一级摘要, 2: 总括摘要
	parent_id integer,  -- RAPTOR 向上追溯
	chunk_order integer not null default 0,  -- 同一层级内的顺序

	foreign key(library_id) references library(id) on delete cascade,
	foreign key(document_id) references documents(id) on delete cascade,
	foreign key(parent_id) references document_nodes(id) on delete set null
);
CREATE INDEX idx_nodes_library_id ON document_nodes(library_id);
CREATE INDEX idx_nodes_document_id ON document_nodes(document_id);
CREATE INDEX idx_nodes_parent_id ON document_nodes(parent_id);
CREATE INDEX idx_nodes_level ON document_nodes(level);
CREATE INDEX idx_nodes_doc_level_order ON document_nodes(document_id, level, chunk_order);

-- 当 document_nodes 插入新行时，同步更新索引
CREATE TRIGGER doc_nodes_ai AFTER INSERT ON document_nodes BEGIN
  INSERT INTO doc_fts(rowid, tokens, library_id, document_id, level)
    VALUES (new.id, new.content_tokens, new.library_id, new.document_id, new.level);
END;

-- 当 document_nodes 删除行时，同步删除索引
CREATE TRIGGER doc_nodes_ad AFTER DELETE ON document_nodes BEGIN
  INSERT INTO doc_fts(doc_fts, rowid, tokens, library_id, document_id, level)
    VALUES('delete', old.id, old.content_tokens, old.library_id, old.document_id, old.level);
END;

-- 当内容修改时，更新索引
CREATE TRIGGER doc_nodes_au AFTER UPDATE OF content_tokens, library_id, document_id, level ON document_nodes BEGIN
  INSERT INTO doc_fts(doc_fts, rowid, tokens, library_id, document_id, level)
    VALUES('delete', old.id, old.content_tokens, old.library_id, old.document_id, old.level);
  INSERT INTO doc_fts(rowid, tokens, library_id, document_id, level)
    VALUES (new.id, new.content_tokens, new.library_id, new.document_id, new.level);
END;

-- 当 documents 插入新行时，同步更新文件名索引
CREATE TRIGGER documents_ai AFTER INSERT ON documents BEGIN
  INSERT INTO doc_name_fts(rowid, name_tokens, library_id, document_id)
    VALUES (new.id, new.name_tokens, new.library_id, new.id);
END;

-- 当 documents 删除行时，同步删除文件名索引
CREATE TRIGGER documents_ad AFTER DELETE ON documents BEGIN
  INSERT INTO doc_name_fts(doc_name_fts, rowid, name_tokens, library_id, document_id)
    VALUES('delete', old.id, old.name_tokens, old.library_id, old.id);
END;

-- 当 documents 更新时，同步更新文件名索引
CREATE TRIGGER documents_au AFTER UPDATE OF name_tokens, library_id ON documents BEGIN
  INSERT INTO doc_name_fts(doc_name_fts, rowid, name_tokens, library_id, document_id)
    VALUES('delete', old.id, old.name_tokens, old.library_id, old.id);
  INSERT INTO doc_name_fts(rowid, name_tokens, library_id, document_id)
    VALUES (new.id, new.name_tokens, new.library_id, new.id);
END;
`

			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP TRIGGER IF EXISTS documents_au;
DROP TRIGGER IF EXISTS documents_ad;
DROP TRIGGER IF EXISTS documents_ai;
DROP TRIGGER IF EXISTS doc_nodes_au;
DROP TRIGGER IF EXISTS doc_nodes_ad;
DROP TRIGGER IF EXISTS doc_nodes_ai;
DROP TABLE IF EXISTS document_nodes;
DROP TABLE IF EXISTS doc_vec;
DROP TABLE IF EXISTS doc_name_fts;
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
