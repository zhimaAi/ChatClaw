// Package native marks the ChatClaw core (non-OpenClaw) side of the app.
// Active native-generated files live under define.NativeDataRootDir ($HOME/.chatclaw/native).
package native

import "chatclaw/internal/define"

// DataRootDir returns the native data root directory.
func DataRootDir() (string, error) {
	return define.NativeDataRootDir()
}
