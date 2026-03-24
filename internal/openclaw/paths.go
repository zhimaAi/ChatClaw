// Package openclaw is the OpenClaw Gateway integration boundary (runtime, agents services).
// Generated OpenClaw state lives under define.OpenClawDataRootDir ($HOME/.chatclaw/openclaw).
package openclaw

import "chatclaw/internal/define"

// DataRootDir returns the OpenClaw integration data root directory.
func DataRootDir() (string, error) {
	return define.OpenClawDataRootDir()
}
