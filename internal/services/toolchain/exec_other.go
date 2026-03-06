//go:build !windows

package toolchain

import "os/exec"

func hideWindow(_ *exec.Cmd) {}
