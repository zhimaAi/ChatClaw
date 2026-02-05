package library

import (
	"context"
	"time"

	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
)

// Library 知识库 DTO（暴露给前端）
type Library struct {
	ID int64 `json:"id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name string `json:"name"`

	SemanticSegmentProviderID string `json:"semantic_segment_provider_id"`
	SemanticSegmentModelID    string `json:"semantic_segment_model_id"`

	TopK           int     `json:"top_k"`
	ChunkSize      int     `json:"chunk_size"`
	ChunkOverlap   int     `json:"chunk_overlap"`
	SortOrder      int     `json:"sort_order"`
}

// CreateLibraryInput 创建知识库的输入参数
// 说明：带默认值的字段前端可不填（后端会用默认值兜底）。
type CreateLibraryInput struct {
	Name string `json:"name"`

	SemanticSegmentProviderID string `json:"semantic_segment_provider_id"`
	SemanticSegmentModelID    string `json:"semantic_segment_model_id"`

	TopK           *int     `json:"top_k"`
	ChunkSize      *int     `json:"chunk_size"`
	ChunkOverlap   *int     `json:"chunk_overlap"`
}

// UpdateLibraryInput 更新知识库的输入参数
type UpdateLibraryInput struct {
	Name *string `json:"name"`

	SemanticSegmentProviderID *string `json:"semantic_segment_provider_id"`
	SemanticSegmentModelID    *string `json:"semantic_segment_model_id"`

	TopK           *int     `json:"top_k"`
	ChunkSize      *int     `json:"chunk_size"`
	ChunkOverlap   *int     `json:"chunk_overlap"`
}

// libraryModel 数据库模型
type libraryModel struct {
	bun.BaseModel `bun:"table:library,alias:l"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	Name string `bun:"name,notnull"`

	SemanticSegmentProviderID string `bun:"semantic_segment_provider_id,notnull"`
	SemanticSegmentModelID    string `bun:"semantic_segment_model_id,notnull"`

	TopK           int     `bun:"top_k,notnull"`
	ChunkSize      int     `bun:"chunk_size,notnull"`
	ChunkOverlap   int     `bun:"chunk_overlap,notnull"`
	SortOrder      int     `bun:"sort_order,notnull"`
}

// BeforeInsert 在 INSERT 时自动设置 created_at 和 updated_at（字符串格式）
var _ bun.BeforeInsertHook = (*libraryModel)(nil)

func (*libraryModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

// BeforeUpdate 在 UPDATE 时自动设置 updated_at（字符串格式）
var _ bun.BeforeUpdateHook = (*libraryModel)(nil)

func (*libraryModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (m *libraryModel) toDTO() Library {
	return Library{
		ID:        m.ID,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,

		Name: m.Name,

		SemanticSegmentProviderID: m.SemanticSegmentProviderID,
		SemanticSegmentModelID:    m.SemanticSegmentModelID,

		TopK:           m.TopK,
		ChunkSize:      m.ChunkSize,
		ChunkOverlap:   m.ChunkOverlap,
		SortOrder:      m.SortOrder,
	}
}
