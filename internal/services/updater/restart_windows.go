//go:build windows

package updater

import (
	"os/exec"
	"syscall"
	"unsafe"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGetFileAttrs  = kernel32.NewProc("GetFileAttributesW")
	procSetFileAttrs  = kernel32.NewProc("SetFileAttributesW")
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

// unhideFile clears the FILE_ATTRIBUTE_HIDDEN flag on a file so that
// os.Remove can delete it reliably. Errors are silently ignored.
func unhideFile(path string) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}

	attrs, _, _ := procGetFileAttrs.Call(uintptr(unsafe.Pointer(pathPtr)))
	if attrs == 0xFFFFFFFF { // INVALID_FILE_ATTRIBUTES — file not found
		return
	}

	// Clear FILE_ATTRIBUTE_HIDDEN (0x2)
	if attrs&0x2 != 0 {
		_, _, _ = procSetFileAttrs.Call(uintptr(unsafe.Pointer(pathPtr)), attrs&^0x2)
	}
}
