//go:build windows

package openclawruntime

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// getOccupyingProcessPID returns the PID of the process listening on the given port.
// Returns 0 if no process is found.
func getOccupyingProcessPID(port int) int {
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	setCmdHideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	suffix := fmt.Sprintf(":%d", port)

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if !strings.EqualFold(fields[0], "TCP") {
			continue
		}
		if !strings.EqualFold(fields[len(fields)-2], "LISTENING") {
			continue
		}
		localAddr := fields[1]
		if !strings.HasSuffix(localAddr, suffix) {
			continue
		}
		pidStr := fields[len(fields)-1]
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 || pid == os.Getpid() {
			continue
		}
		return pid
	}
	return 0
}

// killListenersOnLocalTCPPort kills processes that are LISTENING on 127.0.0.1:port,
// 0.0.0.0:port, or [::1]:port according to netstat. Best-effort; ignores errors from
// taskkill when the PID is already gone.
func killListenersOnLocalTCPPort(port int) error {
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	setCmdHideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("netstat: %w", err)
	}

	suffix := fmt.Sprintf(":%d", port)
	self := os.Getpid()
	seen := make(map[int]struct{})

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if !strings.EqualFold(fields[0], "TCP") {
			continue
		}
		if !strings.EqualFold(fields[len(fields)-2], "LISTENING") {
			continue
		}
		localAddr := fields[1]
		if !strings.HasSuffix(localAddr, suffix) {
			continue
		}
		pidStr := fields[len(fields)-1]
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 || pid == self {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}

		tk := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		setCmdHideWindow(tk)
		_ = tk.Run()
	}
	return nil
}
