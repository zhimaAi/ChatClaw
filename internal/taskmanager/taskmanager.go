package taskmanager

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"maragu.dev/goqite"
	"maragu.dev/goqite/jobs"
)

// 预定义队列名称（可按需扩展）
const (
	QueueThumbnail = "thumbnail" // 快任务：缩略图生成
	QueueDocument  = "document"  // 慢任务：文档解析、向量化
)

// QueueConfig 单个任务队列的配置
type QueueConfig struct {
	Workers      int           // 并发 worker 数量
	PollInterval time.Duration // 轮询新任务的间隔
}

func (c QueueConfig) withDefaults() QueueConfig {
	if c.Workers <= 0 {
		c.Workers = 4
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 100 * time.Millisecond
	}
	return c
}

// Config 任务管理器配置
type Config struct {
	Queues map[string]QueueConfig
}

// TaskManager 基于 goqite 的持久化任务管理器
type TaskManager struct {
	app     *application.App
	db      *sql.DB
	queues  map[string]*taskQueue
	mu      sync.RWMutex
	tasks   map[string]*TaskInfo // taskKey -> TaskInfo（用于取消跟踪）
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	stopped bool
}

// TaskInfo 任务元数据（用于取消）
type TaskInfo struct {
	Key       string // 任务唯一标识
	RunID     string // 运行 ID
	Cancelled bool   // 是否已取消
}

// IsCancelled 检查任务是否应该停止
func (info *TaskInfo) IsCancelled() bool {
	return info == nil || info.Cancelled
}

type taskQueue struct {
	name   string
	queue  *goqite.Queue
	runner *jobs.Runner
}

// JobPayload 序列化的任务数据
type JobPayload struct {
	TaskKey string `json:"task_key"`
	RunID   string `json:"run_id"`
	Data    []byte `json:"data,omitempty"` // 可选的额外数据
}

var (
	once     sync.Once
	instance *TaskManager
)

// Init 初始化全局任务管理器
// app 用于日志和事件发送，sqlDB 应为 bun.DB 的底层 *sql.DB
func Init(app *application.App, sqlDB *sql.DB, cfg Config) error {
	var initErr error
	once.Do(func() {
		if cfg.Queues == nil || len(cfg.Queues) == 0 {
			// 提供默认配置
			cfg.Queues = map[string]QueueConfig{
				QueueThumbnail: {Workers: 8, PollInterval: 50 * time.Millisecond},
				QueueDocument:  {Workers: 2, PollInterval: 100 * time.Millisecond},
			}
		}

		ctx, cancel := context.WithCancel(context.Background())

		tm := &TaskManager{
			app:    app,
			db:     sqlDB,
			queues: make(map[string]*taskQueue, len(cfg.Queues)),
			tasks:  make(map[string]*TaskInfo),
			ctx:    ctx,
			cancel: cancel,
		}

		// 创建每个队列及其 job runner
		for name, qcfg := range cfg.Queues {
			qcfg = qcfg.withDefaults()

			q := goqite.New(goqite.NewOpts{
				DB:   sqlDB,
				Name: name,
			})

			r := jobs.NewRunner(jobs.NewRunnerOpts{
				Limit:        qcfg.Workers,
				Log:          slog.Default(),
				PollInterval: qcfg.PollInterval,
				Queue:        q,
			})

			tm.queues[name] = &taskQueue{
				name:   name,
				queue:  q,
				runner: r,
			}
		}

		instance = tm
	})
	return initErr
}

// Get 返回全局任务管理器实例
func Get() *TaskManager {
	return instance
}

// RegisterHandler 为指定队列和任务类型注册处理器
// 应在初始化时、Start() 之前调用
func (tm *TaskManager) RegisterHandler(queueName, jobType string, handler func(ctx context.Context, info *TaskInfo, data []byte) error) {
	q, ok := tm.queues[queueName]
	if !ok {
		if tm.app != nil {
			tm.app.Logger.Error("unknown queue for handler registration", "queue", queueName, "jobType", jobType)
		}
		return
	}

	q.runner.Register(jobType, func(ctx context.Context, msg []byte) error {
		var payload JobPayload
		if err := json.Unmarshal(msg, &payload); err != nil {
			if tm.app != nil {
				tm.app.Logger.Error("failed to unmarshal job payload", "queue", queueName, "jobType", jobType, "error", err)
			}
			return nil // 不重试格式错误的任务
		}

		// 检查任务是否已取消/被替换。
		// 注意：goqite 的任务是持久化的，应用重启后 tm.tasks 为空；
		// 此时仍应允许运行队列里的任务，否则会出现“退出后任务不继续”的问题。
		tm.mu.RLock()
		info, exists := tm.tasks[payload.TaskKey]
		tm.mu.RUnlock()

		if !exists {
			// 重启后的 orphan job：注册一个临时的 task 记录，让任务继续执行
			tm.mu.Lock()
			// double-check
			if cur, ok := tm.tasks[payload.TaskKey]; ok {
				info = cur
				exists = true
			} else {
				info = &TaskInfo{
					Key:       payload.TaskKey,
					RunID:     payload.RunID,
					Cancelled: false,
				}
				tm.tasks[payload.TaskKey] = info
				exists = true
			}
			tm.mu.Unlock()
		}
		if info.IsCancelled() {
			// 任务已取消
			tm.removeTask(payload.TaskKey, info)
			return nil
		}
		if info.RunID != payload.RunID {
			// 任务已被新运行替换，跳过旧任务
			return nil
		}

		// 执行处理器
		err := handler(ctx, info, payload.Data)

		// 完成后清理任务记录
		tm.removeTask(payload.TaskKey, info)

		return err
	})
}

// Start 启动所有 job runner，应在注册完所有 handler 后调用
func (tm *TaskManager) Start() {
	for _, q := range tm.queues {
		tq := q // 捕获变量
		tm.wg.Add(1)
		go func() {
			defer tm.wg.Done()
			tq.runner.Start(tm.ctx)
		}()
	}
}

// Submit 提交任务到指定队列
// queueName: 预定义的队列常量之一
// jobType: 已注册的任务类型名称
// taskKey: 唯一标识；提交相同 key 会取消之前的任务
// runID: 版本标识，用于检测过期任务
// data: 可选的负载数据
func (tm *TaskManager) Submit(queueName, jobType, taskKey, runID string, data []byte) bool {
	if tm == nil {
		return false
	}

	q, ok := tm.queues[queueName]
	if !ok {
		if tm.app != nil {
			tm.app.Logger.Error("unknown task queue", "queue", queueName, "taskKey", taskKey)
		}
		return false
	}

	// 注册/替换任务记录
	tm.mu.Lock()
	if tm.stopped {
		tm.mu.Unlock()
		return false
	}
	if existing, ok := tm.tasks[taskKey]; ok {
		existing.Cancelled = true
	}
	info := &TaskInfo{
		Key:       taskKey,
		RunID:     runID,
		Cancelled: false,
	}
	tm.tasks[taskKey] = info
	tm.mu.Unlock()

	// 创建任务负载
	payload := JobPayload{
		TaskKey: taskKey,
		RunID:   runID,
		Data:    data,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		if tm.app != nil {
			tm.app.Logger.Error("failed to marshal job payload", "taskKey", taskKey, "error", err)
		}
		return false
	}

	// 提交到 goqite
	if err := jobs.Create(tm.ctx, q.queue, jobType, payloadBytes); err != nil {
		if tm.app != nil {
			tm.app.Logger.Error("failed to create job", "queue", queueName, "jobType", jobType, "taskKey", taskKey, "error", err)
		}
		// 失败时移除任务记录
		tm.mu.Lock()
		if cur, ok := tm.tasks[taskKey]; ok && cur == info {
			delete(tm.tasks, taskKey)
		}
		tm.mu.Unlock()
		return false
	}

	return true
}

// Cancel 通过 taskKey 将任务标记为已取消
func (tm *TaskManager) Cancel(taskKey string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if info, ok := tm.tasks[taskKey]; ok {
		info.Cancelled = true
	}
}

// IsTaskRunning 检查任务是否正在运行
func (tm *TaskManager) IsTaskRunning(taskKey string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	_, ok := tm.tasks[taskKey]
	return ok
}

// GetTaskInfo 返回指定 taskKey 的任务信息
func (tm *TaskManager) GetTaskInfo(taskKey string) *TaskInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.tasks[taskKey]
}

// Emit 发送事件到前端
func (tm *TaskManager) Emit(eventName string, data any) {
	if tm.app != nil {
		tm.app.Event.Emit(eventName, data)
	}
}

// removeTask 移除任务记录（仅当仍指向同一 info 时）
func (tm *TaskManager) removeTask(taskKey string, info *TaskInfo) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if cur, ok := tm.tasks[taskKey]; ok && cur == info {
		delete(tm.tasks, taskKey)
	}
}

// Stop 优雅停止所有 job runner 并等待完成
func (tm *TaskManager) Stop() {
	tm.mu.Lock()
	if tm.stopped {
		tm.mu.Unlock()
		return
	}
	tm.stopped = true
	tm.mu.Unlock()

	tm.cancel()
	tm.wg.Wait()
}

// StopNow 立即停止所有 job runner
func (tm *TaskManager) StopNow() {
	tm.mu.Lock()
	if tm.stopped {
		tm.mu.Unlock()
		return
	}
	tm.stopped = true
	for _, info := range tm.tasks {
		info.Cancelled = true
	}
	tm.mu.Unlock()

	tm.cancel()
	tm.wg.Wait()
}
