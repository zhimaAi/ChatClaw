package library

import (
	"context"
	"time"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

// Folder 文件夹 DTO（暴露给前端）
type Folder struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	LibraryID int64   `json:"library_id"`
	ParentID  *int64  `json:"parent_id"` // nil 表示根文件夹
	Name      string  `json:"name"`
	SortOrder int     `json:"sort_order"`
	Children  []Folder `json:"children,omitempty"` // 子文件夹列表（前端使用）
}

// CreateFolderInput 创建文件夹的输入参数
type CreateFolderInput struct {
	LibraryID int64   `json:"library_id"`
	ParentID  *int64  `json:"parent_id"` // nil 表示根文件夹
	Name      string  `json:"name"`
}

// RenameFolderInput 重命名文件夹的输入参数
type RenameFolderInput struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// DeleteFolderInput 删除文件夹的输入参数
type DeleteFolderInput struct {
	ID int64 `json:"id"`
	// Mode 保留扩展字段，未来如需支持"连文档一并删除"可复用
	Mode string `json:"mode,omitempty"`
}

// MoveDocumentToFolderInput 移动文档到文件夹的输入参数
type MoveDocumentToFolderInput struct {
	DocumentID int64  `json:"document_id"`
	FolderID   *int64 `json:"folder_id"` // nil 或 0 表示移到"未分组"
}

// MoveFolderInput 移动文件夹的输入参数
type MoveFolderInput struct {
	ID       int64  `json:"id"`        // 要移动的文件夹ID
	ParentID *int64 `json:"parent_id"` // nil 表示移到根目录
}

// libraryFolderModel 数据库模型
type libraryFolderModel struct {
	bun.BaseModel `bun:"table:library_folders,alias:lf"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	LibraryID int64   `bun:"library_id,notnull"`
	ParentID  *int64  `bun:"parent_id"` // nil 表示根文件夹
	Name      string  `bun:"name,notnull"`
	SortOrder int     `bun:"sort_order,notnull"`
}

// BeforeInsert 在 INSERT 时自动设置 created_at 和 updated_at
var _ bun.BeforeInsertHook = (*libraryFolderModel)(nil)

func (*libraryFolderModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

// BeforeUpdate 在 UPDATE 时自动设置 updated_at
var _ bun.BeforeUpdateHook = (*libraryFolderModel)(nil)

func (*libraryFolderModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (m *libraryFolderModel) toDTO() Folder {
	return Folder{
		ID:        m.ID,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
		LibraryID: m.LibraryID,
		ParentID:  m.ParentID,
		Name:      m.Name,
		SortOrder: m.SortOrder,
		Children:  []Folder{},
	}
}
