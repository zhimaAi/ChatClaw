package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
)

const (
	// maxFileSize is the maximum size of a single log file (10 MB).
	maxFileSize = 10 * 1024 * 1024
	// maxBackups is the maximum number of rotated log files to keep.
	maxBackups = 5
	// logFileName is the name of the current log file.
	logFileName = "app.log"
	// logDirName is the subdirectory under the app config dir for logs.
	logDirName = "logs"
)

// rotatingWriter is an io.Writer that writes to a file and rotates when the
// file exceeds maxFileSize. Old log files are kept up to maxBackups.
type rotatingWriter struct {
	mu       sync.Mutex
	file     *os.File
	dir      string
	size     int64
	maxSize  int64
	maxFiles int
}

// newRotatingWriter creates a new rotatingWriter and opens the log file.
func newRotatingWriter(dir string) (*rotatingWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	w := &rotatingWriter{
		dir:      dir,
		maxSize:  maxFileSize,
		maxFiles: maxBackups,
	}
	if err := w.openFile(); err != nil {
		return nil, err
	}
	return w, nil
}

// openFile opens (or creates) the current log file and records its size.
func (w *rotatingWriter) openFile() error {
	path := filepath.Join(w.dir, logFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("stat log file: %w", err)
	}
	w.file = f
	w.size = info.Size()
	return nil
}

// Write implements io.Writer. It writes data to the log file and rotates
// the file if it exceeds maxSize.
func (w *rotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Rotate before writing if the file is already too large.
	if w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			// If rotation fails, still try to write to the current file.
			_ = err
		}
	}

	n, err = w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate closes the current log file, renames it with a timestamp suffix,
// removes excess backups, and opens a new log file.
func (w *rotatingWriter) rotate() error {
	if w.file != nil {
		w.file.Close()
	}

	// Rename current log file with timestamp.
	src := filepath.Join(w.dir, logFileName)
	stamp := time.Now().Format("20060102-150405")
	dst := filepath.Join(w.dir, fmt.Sprintf("app-%s.log", stamp))
	if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
		// If rename fails, try to reopen the original file.
		return w.openFile()
	}

	// Remove excess backups.
	w.cleanBackups()

	return w.openFile()
}

// cleanBackups removes the oldest rotated log files if there are more than maxFiles.
func (w *rotatingWriter) cleanBackups() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}

	var backups []string
	for _, e := range entries {
		name := e.Name()
		if name != logFileName && strings.HasPrefix(name, "app-") && strings.HasSuffix(name, ".log") {
			backups = append(backups, name)
		}
	}

	if len(backups) <= w.maxFiles {
		return
	}

	// Sort by name (timestamp in name ensures chronological order).
	sort.Strings(backups)
	// Remove the oldest ones.
	for _, name := range backups[:len(backups)-w.maxFiles] {
		os.Remove(filepath.Join(w.dir, name))
	}
}

// Close closes the underlying file.
func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// --- Public API ---

var (
	globalWriter *rotatingWriter
	globalMu     sync.Mutex
)

// New creates a *slog.Logger that writes to a log file under the app config directory.
//
// Behavior by environment:
//   - Development: writes to both stderr (colored) and file.
//   - Production:  writes to file only (Wails default discards all logs in production).
//
// The returned cleanup function must be called on application shutdown to flush
// and close the log file.
func New() (logger *slog.Logger, cleanup func(), err error) {
	logDir, err := resolveLogDir()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve log dir: %w", err)
	}

	globalMu.Lock()
	defer globalMu.Unlock()

	w, err := newRotatingWriter(logDir)
	if err != nil {
		return nil, nil, fmt.Errorf("init rotating writer: %w", err)
	}
	globalWriter = w

	var writer io.Writer
	if define.IsDev() {
		// Development: dual-write to stderr + file.
		writer = io.MultiWriter(os.Stderr, w)
	} else {
		// Production: file only (console is unavailable in packaged apps).
		writer = w
	}

	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger = slog.New(handler)

	cleanup = func() {
		globalMu.Lock()
		defer globalMu.Unlock()
		if globalWriter != nil {
			globalWriter.Close()
			globalWriter = nil
		}
	}

	return logger, cleanup, nil
}

// resolveLogDir returns the directory for log files: <UserConfigDir>/chatclaw/logs
func resolveLogDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, define.AppID, logDirName), nil
}
