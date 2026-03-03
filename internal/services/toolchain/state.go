package toolchain

import "sync"

// Package-level state: lightweight, lock-protected snapshot of which tools
// are installed and where the bin directory is. Updated by ToolchainService
// after each install/check cycle; read by any package (e.g. agent, chat)
// without needing a reference to ToolchainService itself.

var state struct {
	mu        sync.RWMutex
	binDir    string
	installed map[string]bool // tool name -> installed
}

// SetState updates the package-level toolchain state.
// Called by ToolchainService after detecting or installing tools.
func SetState(binDir string, installed map[string]bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.binDir = binDir
	state.installed = installed
}

// MarkInstalled marks a single tool as installed in the package-level state.
func MarkInstalled(name string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.installed == nil {
		state.installed = make(map[string]bool)
	}
	state.installed[name] = true
}

// BinDirIfReady returns the bin directory only if at least one tool is installed.
func BinDirIfReady() string {
	state.mu.RLock()
	defer state.mu.RUnlock()
	for _, ok := range state.installed {
		if ok {
			return state.binDir
		}
	}
	return ""
}

// IsInstalled returns whether a specific tool is installed.
func IsInstalled(name string) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.installed[name]
}

// InstalledSnapshot returns a copy of the current installed-tools map.
func InstalledSnapshot() map[string]bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.installed == nil {
		return nil
	}
	cp := make(map[string]bool, len(state.installed))
	for k, v := range state.installed {
		cp[k] = v
	}
	return cp
}
