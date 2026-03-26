package define

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// LegacyDataRootDir returns the historical app root: $HOME/.chatclaw
// Files here are legacy layout; EnsureDataLayout migrates them into native/ or openclaw/.
func LegacyDataRootDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "."+AppID), nil
}

// NativeDataRootDir returns the ChatClaw native data root: $HOME/.chatclaw/native
func NativeDataRootDir() (string, error) {
	leg, err := LegacyDataRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(leg, "native"), nil
}

// OpenClawDataRootDir returns the OpenClaw integration data root: $HOME/.chatclaw/openclaw
func OpenClawDataRootDir() (string, error) {
	leg, err := LegacyDataRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(leg, "openclaw"), nil
}

var dataLayoutOnce sync.Once
var dataLayoutErr error

// EnsureDataLayout creates native/openclaw subdirectories and migrates legacy files
// from $HOME/.chatclaw into them once. Safe to call from multiple goroutines.
func EnsureDataLayout() error {
	dataLayoutOnce.Do(func() { dataLayoutErr = ensureDataLayout() })
	return dataLayoutErr
}

func ensureDataLayout() error {
	legacy, err := LegacyDataRootDir()
	if err != nil {
		return err
	}
	native, err := NativeDataRootDir()
	if err != nil {
		return err
	}
	oc, err := OpenClawDataRootDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(legacy, 0o755); err != nil {
		return fmt.Errorf("legacy data root: %w", err)
	}
	if err := os.MkdirAll(native, 0o755); err != nil {
		return fmt.Errorf("native data root: %w", err)
	}
	if err := os.MkdirAll(oc, 0o700); err != nil {
		return fmt.Errorf("openclaw data root: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(oc, "runtime"), 0o700); err != nil {
		return fmt.Errorf("openclaw runtime root: %w", err)
	}

	if err := migrateFileIfMissing(filepath.Join(native, DefaultSQLiteFileName), filepath.Join(legacy, DefaultSQLiteFileName)); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	if err := migrateFileIfMissing(filepath.Join(oc, "openclaw.json"), filepath.Join(legacy, "openclaw.json")); err != nil {
		return fmt.Errorf("migrate openclaw.json: %w", err)
	}
	if err := migrateDirTreeIfMissing(filepath.Join(oc, "identity"), filepath.Join(legacy, "identity")); err != nil {
		return fmt.Errorf("migrate identity: %w", err)
	}
	if err := migrateDirTreeIfMissing(filepath.Join(oc, "agents"), filepath.Join(legacy, "agents")); err != nil {
		return fmt.Errorf("migrate openclaw agents dir: %w", err)
	}
	if err := migrateWorkspaceDirs(legacy, oc); err != nil {
		return fmt.Errorf("migrate workspace dirs: %w", err)
	}
	if err := migrateSplitLogs(filepath.Join(legacy, "logs"), filepath.Join(native, "logs"), filepath.Join(oc, "logs")); err != nil {
		return fmt.Errorf("migrate logs: %w", err)
	}
	if err := migrateDirTreeIfMissing(filepath.Join(native, "skills"), filepath.Join(legacy, "skills")); err != nil {
		return fmt.Errorf("migrate skills: %w", err)
	}
	if err := migrateDirTreeIfMissing(filepath.Join(native, "mcp"), filepath.Join(legacy, "mcp")); err != nil {
		return fmt.Errorf("migrate mcp: %w", err)
	}
	return nil
}

func migrateFileIfMissing(dst, src string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return copyFile(dst, src, 0o644)
}

func migrateDirTreeIfMissing(dstRoot, srcRoot string) error {
	if fi, err := os.Stat(dstRoot); err == nil && fi.IsDir() {
		if entries, err := os.ReadDir(dstRoot); err == nil && len(entries) > 0 {
			return nil
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(srcRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstRoot), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dstRoot); err != nil && !os.IsNotExist(err) {
		return err
	}
	return copyDirRecursive(dstRoot, srcRoot)
}

func migrateWorkspaceDirs(legacyRoot, ocRoot string) error {
	pattern := filepath.Join(legacyRoot, "workspace-*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, src := range matches {
		base := filepath.Base(src)
		dst := filepath.Join(ocRoot, base)
		if _, err := os.Stat(dst); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if _, err := os.Stat(src); err != nil {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			if err := copyDirRecursive(dst, src); err != nil {
				return fmt.Errorf("move workspace %s: %w", base, err)
			}
			_ = os.RemoveAll(src)
		}
	}
	return nil
}

func migrateSplitLogs(legacyLogs, nativeLogs, ocLogs string) error {
	if _, err := os.Stat(legacyLogs); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	entries, err := os.ReadDir(legacyLogs)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		src := filepath.Join(legacyLogs, name)
		isGateway := strings.Contains(strings.ToLower(name), "openclaw-gateway")
		dstDir := nativeLogs
		if isGateway {
			dstDir = ocLogs
		}
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return err
		}
		dst := filepath.Join(dstDir, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := copyFile(dst, src, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(dst, src string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

func copyDirRecursive(dstRoot, srcRoot string) error {
	return filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstRoot, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(target, path, info.Mode().Perm())
	})
}
