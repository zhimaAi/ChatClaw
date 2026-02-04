package document

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"willchat/internal/define"
	"willchat/internal/errs"
	"willchat/internal/services/thumbnail"
	"willchat/internal/sqlite"
	"willchat/internal/taskmanager"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Job type constants for document tasks.
const (
	JobTypeThumbnail = "thumbnail" // Generate document thumbnail
	JobTypeProcess   = "process"   // Parse and embed document
)

// ThumbnailJobData holds data for thumbnail generation job.
type ThumbnailJobData struct {
	DocID     int64  `json:"doc_id"`
	LibraryID int64  `json:"library_id"`
	LocalPath string `json:"local_path"`
}

// ProcessJobData holds data for document processing job.
type ProcessJobData struct {
	DocID     int64  `json:"doc_id"`
	LibraryID int64  `json:"library_id"`
	RunID     string `json:"run_id"`
}

// DocumentService 文档服务（暴露给前端调用）
type DocumentService struct {
	app *application.App
}

func NewDocumentService(app *application.App) *DocumentService {
	return &DocumentService{app: app}
}

// ServiceStartup 实现 Wails 服务生命周期接口
// 在应用启动时注册任务处理器并启动任务管理器
func (s *DocumentService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.registerTaskHandlers()
	taskmanager.Get().Start()
	return nil
}

// registerTaskHandlers registers document-related job handlers with the task manager.
func (s *DocumentService) registerTaskHandlers() {
	tm := taskmanager.Get()
	if tm == nil {
		return
	}

	// Register thumbnail generation handler
	tm.RegisterHandler(taskmanager.QueueThumbnail, JobTypeThumbnail, func(ctx context.Context, info *taskmanager.TaskInfo, data []byte) error {
		var jobData ThumbnailJobData
		if err := json.Unmarshal(data, &jobData); err != nil {
			if s.app != nil {
				s.app.Logger.Error("failed to unmarshal thumbnail job data", "error", err)
			}
			return nil // Don't retry malformed jobs
		}
		s.generateThumbnail(jobData.DocID, jobData.LibraryID, jobData.LocalPath, info)
		return nil
	})

	// Register document processing handler
	tm.RegisterHandler(taskmanager.QueueDocument, JobTypeProcess, func(ctx context.Context, info *taskmanager.TaskInfo, data []byte) error {
		var jobData ProcessJobData
		if err := json.Unmarshal(data, &jobData); err != nil {
			if s.app != nil {
				s.app.Logger.Error("failed to unmarshal process job data", "error", err)
			}
			return nil // Don't retry malformed jobs
		}
		s.processDocument(jobData.DocID, jobData.LibraryID, jobData.RunID, info)
		return nil
	})
}

func (s *DocumentService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// GetDocumentsDir 获取文档存储目录
func (s *DocumentService) GetDocumentsDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", errs.Wrap("error.document_dir_failed", err)
	}
	return filepath.Join(cfgDir, define.AppID, "documents"), nil
}

// ListDocuments 获取知识库的文档列表
// keyword: 可选的搜索关键词（按文件名搜索）
func (s *DocumentService) ListDocuments(libraryID int64, keyword string) ([]Document, error) {
	if libraryID <= 0 {
		return nil, errs.New("error.library_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models := make([]documentModel, 0)
	q := db.NewSelect().Model(&models).Where("library_id = ?", libraryID)

	// 如果有搜索关键词，使用 LIKE 进行简单搜索
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		q = q.Where("original_name LIKE ?", "%"+keyword+"%")
	}

	if err := q.OrderExpr("id DESC").Scan(ctx); err != nil {
		return nil, errs.Wrap("error.document_list_failed", err)
	}

	out := make([]Document, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

// UploadDocuments 上传文档
func (s *DocumentService) UploadDocuments(input UploadInput) ([]Document, error) {
	if input.LibraryID <= 0 {
		return nil, errs.New("error.library_id_required")
	}
	if len(input.FilePaths) == 0 {
		return nil, errs.New("error.document_file_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	docsDir, err := s.GetDocumentsDir()
	if err != nil {
		return nil, err
	}

	// 确保目录存在
	libraryDir := filepath.Join(docsDir, fmt.Sprintf("%d", input.LibraryID))
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		return nil, errs.Wrap("error.document_upload_failed", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	uploaded := make([]Document, 0, len(input.FilePaths))

	for _, srcPath := range input.FilePaths {
		doc, err := s.uploadSingleFile(ctx, db, input.LibraryID, libraryDir, srcPath)
		if err != nil {
			// 记录错误但继续处理其他文件
			if s.app != nil {
				s.app.Logger.Warn("upload file failed", "path", srcPath, "error", err)
			}
			continue
		}
		uploaded = append(uploaded, *doc)

		// 启动异步处理任务
		s.startProcessingTask(doc)

		// 启动缩略图生成任务
		s.startThumbnailTask(doc)
	}

	if len(uploaded) == 0 {
		return nil, errs.New("error.document_upload_failed")
	}

	return uploaded, nil
}

// uploadSingleFile 上传单个文件
func (s *DocumentService) uploadSingleFile(ctx context.Context, db *bun.DB, libraryID int64, libraryDir, srcPath string) (*Document, error) {
	// 检查文件是否存在
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	// 获取文件扩展名（去掉小数点前缀）
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(srcPath)), ".")
	if !IsSupportedExtension(ext) {
		return nil, errs.Newf("error.document_file_type_not_supported", map[string]any{"Ext": ext})
	}

	// 计算文件 hash
	hash, err := s.calculateFileHash(srcPath)
	if err != nil {
		return nil, fmt.Errorf("calculate hash: %w", err)
	}

	// 检查是否已存在相同文件，如果存在则删除旧记录（覆盖上传）
	var existingDoc documentModel
	err = db.NewSelect().
		Model(&existingDoc).
		Where("library_id = ?", libraryID).
		Where("content_hash = ?", hash).
		Scan(ctx)
	if err == nil {
		// 存在相同文件，取消旧任务并删除旧记录和文件
		if tm := taskmanager.Get(); tm != nil {
			tm.Cancel(fmt.Sprintf("doc:%d", existingDoc.ID))
			tm.Cancel(fmt.Sprintf("thumb:%d", existingDoc.ID))
		}
		// 删除旧文件
		if existingDoc.LocalPath != "" {
			os.Remove(existingDoc.LocalPath)
		}
		// 删除旧记录
		db.NewDelete().Model(&existingDoc).Where("id = ?", existingDoc.ID).Exec(ctx)
	}

	// 生成目标文件名：hash_原始文件名
	originalName := filepath.Base(srcPath)
	destName := fmt.Sprintf("%s_%s", hash[:8], originalName)
	destPath := filepath.Join(libraryDir, destName)

	// 复制文件
	if err := s.copyFile(srcPath, destPath); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}

	// 生成运行 ID
	runID := uuid.New().String()

	// 插入数据库记录
	m := &documentModel{
		LibraryID:       libraryID,
		OriginalName:    originalName,
		NameTokens:      "", // TODO: 分词
		ThumbIcon:       "",
		FileSize:        srcInfo.Size(),
		ContentHash:     hash,
		Extension:       ext,
		MimeType:        GetMimeType(ext),
		SourceType:      "local",
		LocalPath:       destPath,
		ProcessingRunID: runID,
		ParsingStatus:   StatusPending,
		EmbeddingStatus: StatusPending,
	}

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		// 插入失败，删除已复制的文件
		os.Remove(destPath)
		return nil, fmt.Errorf("insert record: %w", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// RenameDocument 重命名文档
func (s *DocumentService) RenameDocument(input RenameInput) (*Document, error) {
	if input.ID <= 0 {
		return nil, errs.New("error.document_id_required")
	}
	newName := strings.TrimSpace(input.NewName)
	if newName == "" {
		return nil, errs.New("error.document_name_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 查询文档
	var m documentModel
	if err := db.NewSelect().Model(&m).Where("id = ?", input.ID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.document_not_found", map[string]any{"ID": input.ID})
		}
		return nil, errs.Wrap("error.document_read_failed", err)
	}

	// 获取新的扩展名（保持原扩展名）
	oldExt := filepath.Ext(m.OriginalName)
	newExt := filepath.Ext(newName)
	if newExt == "" {
		newName = newName + oldExt
	}

	// 重命名物理文件
	if m.LocalPath != "" {
		oldPath := m.LocalPath
		dir := filepath.Dir(oldPath)
		// 保持 hash 前缀
		hashPrefix := m.ContentHash[:8]
		newFileName := fmt.Sprintf("%s_%s", hashPrefix, newName)
		newPath := filepath.Join(dir, newFileName)

		if err := os.Rename(oldPath, newPath); err != nil {
			if !os.IsNotExist(err) {
				return nil, errs.Wrap("error.document_rename_failed", err)
			}
			// 文件不存在，仍然更新数据库
		}
		m.LocalPath = newPath
	}

	// 更新数据库
	m.OriginalName = newName
	if _, err := db.NewUpdate().Model(&m).
		Column("original_name", "local_path", "updated_at").
		Where("id = ?", input.ID).
		Exec(ctx); err != nil {
		return nil, errs.Wrap("error.document_rename_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// DeleteDocument 删除文档
func (s *DocumentService) DeleteDocument(id int64) error {
	if id <= 0 {
		return errs.New("error.document_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 查询文档
	var m documentModel
	if err := db.NewSelect().Model(&m).Where("id = ?", id).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.Newf("error.document_not_found", map[string]any{"ID": id})
		}
		return errs.Wrap("error.document_read_failed", err)
	}

	// 取消正在进行的任务
	if tm := taskmanager.Get(); tm != nil {
		tm.Cancel(fmt.Sprintf("doc:%d", id))
		tm.Cancel(fmt.Sprintf("thumb:%d", id))
	}

	// 删除物理文件（即使失败也继续删除数据库记录）
	if m.LocalPath != "" {
		if err := os.Remove(m.LocalPath); err != nil && !os.IsNotExist(err) {
			if s.app != nil {
				s.app.Logger.Warn("delete file failed", "path", m.LocalPath, "error", err)
			}
		}
	}

	// 删除数据库记录（CASCADE 会自动删除 document_nodes）
	if _, err := db.NewDelete().Model(&m).Where("id = ?", id).Exec(ctx); err != nil {
		return errs.Wrap("error.document_delete_failed", err)
	}

	return nil
}

// calculateFileHash 计算文件 SHA256 哈希
func (s *DocumentService) calculateFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile 复制文件
func (s *DocumentService) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// startProcessingTask 启动文档处理任务
func (s *DocumentService) startProcessingTask(doc *Document) {
	tm := taskmanager.Get()
	if tm == nil {
		return
	}

	taskKey := fmt.Sprintf("doc:%d", doc.ID)
	runID := doc.ProcessingRunID

	jobData, _ := json.Marshal(ProcessJobData{
		DocID:     doc.ID,
		LibraryID: doc.LibraryID,
		RunID:     runID,
	})

	tm.Submit(taskmanager.QueueDocument, JobTypeProcess, taskKey, runID, jobData)
}

// startThumbnailTask 启动缩略图生成任务
func (s *DocumentService) startThumbnailTask(doc *Document) {
	tm := taskmanager.Get()
	if tm == nil {
		return
	}

	// 使用独立的任务 key，避免与文档处理任务冲突
	taskKey := fmt.Sprintf("thumb:%d", doc.ID)

	jobData, _ := json.Marshal(ThumbnailJobData{
		DocID:     doc.ID,
		LibraryID: doc.LibraryID,
		LocalPath: doc.LocalPath,
	})

	tm.Submit(taskmanager.QueueThumbnail, JobTypeThumbnail, taskKey, doc.ProcessingRunID, jobData)
}

// generateThumbnail 生成文档缩略图
func (s *DocumentService) generateThumbnail(docID, libraryID int64, localPath string, info *taskmanager.TaskInfo) {
	if info.IsCancelled() {
		return
	}

	tm := taskmanager.Get()
	db, err := s.db()
	if err != nil {
		return
	}

	ctx := context.Background()

	// 生成缩略图
	result := thumbnail.Generate(localPath)

	// 更新数据库
	thumbIcon := result.DataURI
	if thumbIcon != "" {
		_, err = db.NewUpdate().
			Table("documents").
			Set("thumb_icon = ?", thumbIcon).
			Set("updated_at = ?", sqlite.NowUTC()).
			Where("id = ?", docID).
			Exec(ctx)

		if err != nil {
			if s.app != nil {
				s.app.Logger.Warn("failed to update thumbnail", "docID", docID, "error", err)
			}
			return
		}
	}

	// 发送事件通知前端
	if tm != nil {
		tm.Emit("document:thumbnail", ThumbnailEvent{
			DocumentID: docID,
			LibraryID:  libraryID,
			ThumbIcon:  thumbIcon,
		})
	}
}

// processDocument 处理文档（解析 + 向量化）
func (s *DocumentService) processDocument(docID, libraryID int64, runID string, info *taskmanager.TaskInfo) {
	tm := taskmanager.Get()
	db, err := s.db()
	if err != nil {
		return
	}

	ctx := context.Background()

	// 辅助函数：检查任务是否应该继续
	shouldContinue := func() bool {
		if info.IsCancelled() {
			return false
		}
		// 检查 runID 是否匹配
		var currentRunID string
		if err := db.NewSelect().
			Table("documents").
			Column("processing_run_id").
			Where("id = ?", docID).
			Scan(ctx, &currentRunID); err != nil {
			return false
		}
		return currentRunID == runID
	}

	// 辅助函数：更新状态并发送事件
	updateAndEmit := func(parsingStatus, parsingProgress int, parsingError string, embeddingStatus, embeddingProgress int, embeddingError string) {
		db.NewUpdate().
			Table("documents").
			Set("parsing_status = ?", parsingStatus).
			Set("parsing_progress = ?", parsingProgress).
			Set("parsing_error = ?", parsingError).
			Set("embedding_status = ?", embeddingStatus).
			Set("embedding_progress = ?", embeddingProgress).
			Set("embedding_error = ?", embeddingError).
			Set("updated_at = ?", sqlite.NowUTC()).
			Where("id = ?", docID).
			Where("processing_run_id = ?", runID). // 只更新当前运行的任务
			Exec(ctx)

		if tm != nil {
			tm.Emit("document:progress", ProgressEvent{
				DocumentID:        docID,
				LibraryID:         libraryID,
				ParsingStatus:     parsingStatus,
				ParsingProgress:   parsingProgress,
				ParsingError:      parsingError,
				EmbeddingStatus:   embeddingStatus,
				EmbeddingProgress: embeddingProgress,
				EmbeddingError:    embeddingError,
			})
		}
	}

	// ========== 阶段1: 解析文档 ==========
	if !shouldContinue() {
		return
	}
	updateAndEmit(StatusProcessing, 0, "", StatusPending, 0, "")

	// 模拟解析过程
	for progress := 10; progress <= 100; progress += 10 {
		if !shouldContinue() {
			return
		}
		time.Sleep(200 * time.Millisecond) // 模拟耗时

		// 随机模拟解析失败（约 30% 概率）
		if progress == 50 && rand.Intn(10) < 3 {
			updateAndEmit(StatusFailed, progress, "模拟解析失败：文档格式不支持", StatusPending, 0, "")
			return
		}

		updateAndEmit(StatusProcessing, progress, "", StatusPending, 0, "")
	}

	// 解析完成
	if !shouldContinue() {
		return
	}
	updateAndEmit(StatusCompleted, 100, "", StatusPending, 0, "")

	// ========== 阶段2: 向量化 ==========
	if !shouldContinue() {
		return
	}
	updateAndEmit(StatusCompleted, 100, "", StatusProcessing, 0, "")

	// 模拟向量化过程
	for progress := 10; progress <= 100; progress += 10 {
		if !shouldContinue() {
			return
		}
		time.Sleep(200 * time.Millisecond) // 模拟耗时

		// 随机模拟向量化失败（约 30% 概率）
		if progress == 70 && rand.Intn(10) < 3 {
			updateAndEmit(StatusCompleted, 100, "", StatusFailed, progress, "模拟向量化失败：嵌入模型调用异常")
			return
		}

		updateAndEmit(StatusCompleted, 100, "", StatusProcessing, progress, "")
	}

	// 全部完成
	if !shouldContinue() {
		return
	}
	updateAndEmit(StatusCompleted, 100, "", StatusCompleted, 100, "")
}
