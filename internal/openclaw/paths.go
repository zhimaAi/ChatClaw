// Package openclaw is the OpenClaw Gateway integration boundary (runtime, agents services).
// Generated OpenClaw state lives under define.OpenClawDataRootDir ($HOME/.chatclaw/openclaw).
package openclaw

import (
	"path/filepath"

	"chatclaw/internal/define"
)

// DataRootDir returns the OpenClaw integration data root directory.
func DataRootDir() (string, error) {
	return define.OpenClawDataRootDir()
}

// UserRuntimeRootDir returns the root directory for user-managed runtime overrides:
// $HOME/.chatclaw/openclaw/runtime
func UserRuntimeRootDir() (string, error) {
	root, err := DataRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "runtime"), nil
}

// UserRuntimeTargetDir returns the runtime override directory for a specific target:
// $HOME/.chatclaw/openclaw/runtime/<target>
func UserRuntimeTargetDir(target string) (string, error) {
	root, err := UserRuntimeRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, target), nil
}

// UserRuntimeCurrentDir returns the symlink-or-directory path used to point to the
// currently active user-managed runtime override for a specific target:
// $HOME/.chatclaw/openclaw/runtime/<target>/current
func UserRuntimeCurrentDir(target string) (string, error) {
	dir, err := UserRuntimeTargetDir(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "current"), nil
}
