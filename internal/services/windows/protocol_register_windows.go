//go:build windows

package windows

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const chatclawProtocolDesc = "URL:ChatClaw Protocol"

// RegisterChatClawProtocol registers the chatclaw:// URL scheme in the current user's
// registry (HKCU\Software\Classes\chatclaw) so that browsers can launch the app
// after OAuth login. This fixes "scheme does not have a registered handler" on
// machines where the NSIS installer did not run or the registration was lost.
// Safe to call on every startup; no admin rights required.
func RegisterChatClawProtocol() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return err
	}
	// Registry command must use executable path as-is (backslashes on Windows)
	// Command line: "C:\path\to\ChatClaw.exe" "%1"
	command := `"` + exePath + `" "%1"`
	icon := exePath + ",0"

	// HKCU\Software\Classes\chatclaw
	k, err := registry.OpenKey(registry.CURRENT_USER, "Software\\Classes", registry.SET_VALUE|registry.CREATE_SUB_KEY)
	if err != nil {
		return err
	}
	defer k.Close()

	chatclaw, _, err := registry.CreateKey(k, "chatclaw", registry.SET_VALUE|registry.CREATE_SUB_KEY)
	if err != nil {
		return err
	}
	defer chatclaw.Close()

	if err := chatclaw.SetStringValue("", chatclawProtocolDesc); err != nil {
		return err
	}
	if err := chatclaw.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}

	// DefaultIcon
	iconKey, _, err := registry.CreateKey(chatclaw, "DefaultIcon", registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer iconKey.Close()
	if err := iconKey.SetStringValue("", icon); err != nil {
		return err
	}

	// shell\open\command
	shellOpen, _, err := registry.CreateKey(chatclaw, "shell\\open\\command", registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer shellOpen.Close()
	return shellOpen.SetStringValue("", command)
}
