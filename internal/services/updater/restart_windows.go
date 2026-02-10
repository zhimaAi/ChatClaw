//go:build windows

package updater

import (
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGetFileAttrs = kernel32.NewProc("GetFileAttributesW")
	procSetFileAttrs = kernel32.NewProc("SetFileAttributesW")
	procDeleteFile   = kernel32.NewProc("DeleteFileW")
)

// setDetachedProcess configures the command to run without a visible console
// window and in a new process group so it survives parent exit.
//
// CREATE_NO_WINDOW (0x08000000) prevents cmd.exe from showing a console.
// CREATE_NEW_PROCESS_GROUP detaches it from the parent's console group.
func setDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x08000000, // 0x08000000 = CREATE_NO_WINDOW
	}
}

// ntPath converts a regular Windows path to an extended-length NT path using
// the \\?\ prefix. This bypasses the Win32 path parser which interprets a
// leading dot component (e.g. "dir\.file") as a relative directory reference,
// making it impossible to access files whose names start with a dot.
func ntPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	return `\\?\` + path
}

// removeHiddenFile removes a file that may be hidden and whose name starts
// with a dot. On Windows both conditions require special handling:
//   - Hidden attribute must be cleared before some delete APIs work.
//   - The \\?\ prefix is needed so the Win32 API doesn't misinterpret the
//     leading dot as a current-directory reference.
//
// Returns nil on success, or an error describing the failure.
func removeHiddenFile(path string) error {
	extPath := ntPath(path)

	pathPtr, err := syscall.UTF16PtrFromString(extPath)
	if err != nil {
		return err
	}

	// Clear hidden attribute (best-effort; ignore errors).
	attrs, _, _ := procGetFileAttrs.Call(uintptr(unsafe.Pointer(pathPtr)))
	if attrs != 0xFFFFFFFF && attrs&0x2 != 0 {
		_, _, _ = procSetFileAttrs.Call(uintptr(unsafe.Pointer(pathPtr)), attrs&^0x2)
	}

	// Delete via kernel32 DeleteFileW with the extended-length path.
	ret, _, err := procDeleteFile.Call(uintptr(unsafe.Pointer(pathPtr)))
	if ret == 0 { // 0 means failure
		return err
	}
	return nil
}
