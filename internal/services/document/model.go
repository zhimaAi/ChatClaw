package document

import (
	"context"
	"time"

	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
)

// 处理状态常量
const (
	StatusPending    = 0 // 待处理
	StatusProcessing = 1 // 处理中
	StatusCompleted  = 2 // 已完成
	StatusFailed     = 3 // 失败
)

// Document 文档 DTO（暴露给前端）
type Document struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	LibraryID    int64  `json:"library_id"`
	OriginalName string `json:"original_name"`
	ThumbIcon    string `json:"thumb_icon"`
	FileSize     int64  `json:"file_size"`
	ContentHash  string `json:"content_hash"`

	Extension  string `json:"extension"`
	MimeType   string `json:"mime_type"`
	SourceType string `json:"source_type"` // local, web

	LocalPath   string `json:"local_path"`
	WebURL      string `json:"web_url"`
	FileMissing bool   `json:"file_missing"` // 原始文件是否丢失（被用户手动删除）

	ProcessingRunID string `json:"processing_run_id"`

	ParsingStatus   int    `json:"parsing_status"`
	ParsingProgress int    `json:"parsing_progress"`
	ParsingError    string `json:"parsing_error"`

	EmbeddingStatus   int    `json:"embedding_status"`
	EmbeddingProgress int    `json:"embedding_progress"`
	EmbeddingError    string `json:"embedding_error"`

	WordTotal  int `json:"word_total"`
	SplitTotal int `json:"split_total"`
}

// UploadInput 上传文档的输入参数
type UploadInput struct {
	LibraryID int64    `json:"library_id"`
	FilePaths []string `json:"file_paths"`
}

// UploadProgressEvent 上传进度事件（发送给前端）
type UploadProgressEvent struct {
	LibraryID int64 `json:"library_id"`
	Total     int   `json:"total"`
	Done      int   `json:"done"`
}

// RenameInput 重命名文档的输入参数
type RenameInput struct {
	ID      int64  `json:"id"`
	NewName string `json:"new_name"`
}

// ListDocumentsPageInput 文档分页查询输入参数（cursor 分页）
// - BeforeID: 返回 id < before_id 的数据（按 id DESC）
// - Limit: 每次返回条数（建议 100）
// - SortBy: 排序方式（"created_desc" 或 "created_asc"），默认 "created_desc"
type ListDocumentsPageInput struct {
	LibraryID int64  `json:"library_id"`
	Keyword   string `json:"keyword"`
	BeforeID  int64  `json:"before_id"`
	Limit     int    `json:"limit"`
	SortBy    string `json:"sort_by"`
}

// ProgressEvent 进度事件数据（发送给前端）
type ProgressEvent struct {
	DocumentID        int64  `json:"document_id"`
	LibraryID         int64  `json:"library_id"`
	ParsingStatus     int    `json:"parsing_status"`
	ParsingProgress   int    `json:"parsing_progress"`
	ParsingError      string `json:"parsing_error"`
	EmbeddingStatus   int    `json:"embedding_status"`
	EmbeddingProgress int    `json:"embedding_progress"`
	EmbeddingError    string `json:"embedding_error"`
}

// ThumbnailEvent 缩略图更新事件数据（发送给前端）
type ThumbnailEvent struct {
	DocumentID int64  `json:"document_id"`
	LibraryID  int64  `json:"library_id"`
	ThumbIcon  string `json:"thumb_icon"` // base64 data URI or empty
}

// documentModel 数据库模型
type documentModel struct {
	bun.BaseModel `bun:"table:documents,alias:d"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	LibraryID    int64  `bun:"library_id,notnull"`
	OriginalName string `bun:"original_name,notnull"`
	NameTokens   string `bun:"name_tokens,notnull"`
	ThumbIcon    string `bun:"thumb_icon"`
	FileSize     int64  `bun:"file_size,notnull"`
	ContentHash  string `bun:"content_hash,notnull"`

	Extension  string `bun:"extension,notnull"`
	MimeType   string `bun:"mime_type,notnull"`
	SourceType string `bun:"source_type,notnull"`

	LocalPath string `bun:"local_path"`
	WebURL    string `bun:"web_url"`

	ProcessingRunID string `bun:"processing_run_id,notnull"`

	ParsingStatus   int    `bun:"parsing_status,notnull"`
	ParsingProgress int    `bun:"parsing_progress,notnull"`
	ParsingError    string `bun:"parsing_error,notnull"`

	EmbeddingStatus   int    `bun:"embedding_status,notnull"`
	EmbeddingProgress int    `bun:"embedding_progress,notnull"`
	EmbeddingError    string `bun:"embedding_error,notnull"`

	WordTotal  int `bun:"word_total,notnull"`
	SplitTotal int `bun:"split_total,notnull"`
}

// BeforeInsert 在 INSERT 时自动设置 created_at 和 updated_at
var _ bun.BeforeInsertHook = (*documentModel)(nil)

func (*documentModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

// BeforeUpdate 在 UPDATE 时自动设置 updated_at
var _ bun.BeforeUpdateHook = (*documentModel)(nil)

func (*documentModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (m *documentModel) toDTO() Document {
	return Document{
		ID:        m.ID,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,

		LibraryID:    m.LibraryID,
		OriginalName: m.OriginalName,
		ThumbIcon:    m.ThumbIcon,
		FileSize:     m.FileSize,
		ContentHash:  m.ContentHash,

		Extension:  m.Extension,
		MimeType:   m.MimeType,
		SourceType: m.SourceType,

		LocalPath: m.LocalPath,
		WebURL:    m.WebURL,

		ProcessingRunID: m.ProcessingRunID,

		ParsingStatus:   m.ParsingStatus,
		ParsingProgress: m.ParsingProgress,
		ParsingError:    m.ParsingError,

		EmbeddingStatus:   m.EmbeddingStatus,
		EmbeddingProgress: m.EmbeddingProgress,
		EmbeddingError:    m.EmbeddingError,

		WordTotal:  m.WordTotal,
		SplitTotal: m.SplitTotal,
	}
}

// 支持的文件扩展名及其 MIME 类型（不带小数点前缀）
var supportedExtensions = map[string]string{
	"pdf":  "application/pdf",
	"doc":  "application/msword",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"txt":  "text/plain",
	"md":   "text/markdown",
	"csv":  "text/csv",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"html": "text/html",
	"htm":  "text/html",
	"ofd":  "application/ofd",
}

// IsSupportedExtension 检查扩展名是否支持
func IsSupportedExtension(ext string) bool {
	_, ok := supportedExtensions[ext]
	return ok
}

// GetMimeType 获取扩展名对应的 MIME 类型
func GetMimeType(ext string) string {
	if mime, ok := supportedExtensions[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}
