//go:build !windows

package windows

// RegisterChatClawProtocol is a no-op on non-Windows. URL scheme registration
// is only needed on Windows (HKCU); macOS uses Info.plist CFBundleURLTypes.
func RegisterChatClawProtocol() error {
	return nil
}
