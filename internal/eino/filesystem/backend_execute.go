package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
)

// ShellPolicy defines security constraints for shell command execution.
type ShellPolicy struct {
	TrustedDirs     []string      // Allowed working directories. Empty = no restriction.
	BlockedCommands []string      // Rejected command patterns (substring match).
	DefaultTimeout  time.Duration // Max execution time per command. 0 = 60s default.
}

// Execute runs a shell command and returns its output.
// Shell: powershell on Windows, zsh on macOS, bash on Linux.
// Working directory: baseDir.
//
// The command runs in its own process group so that on timeout or cancellation
// the entire process tree (including child processes such as `php artisan serve`)
// is killed immediately, preventing the tool from hanging.
func (b *LocalBackend) Execute(ctx context.Context, req *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	if err := b.validateCommand(req.Command); err != nil {
		exitCode := -1
		return &filesystem.ExecuteResponse{
			Output:   "Command blocked: " + err.Error(),
			ExitCode: &exitCode,
		}, nil
	}

	timeout := 60 * time.Second
	if b.policy != nil && b.policy.DefaultTimeout > 0 {
		timeout = b.policy.DefaultTimeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the command WITHOUT CommandContext — we manage the lifecycle manually
	// via process groups so that all child processes are killed on cancellation.
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Prepend UTF-8 encoding directives so Chinese and other non-ASCII
		// characters in command output are not garbled.  Both the .NET
		// Console encoding and PowerShell's $OutputEncoding must be set
		// because they govern different output paths.
		wrappedCmd := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
			"$OutputEncoding = [System.Text.Encoding]::UTF8; " +
			req.Command
		cmd = exec.Command("powershell.exe",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-Command", wrappedCmd,
		)
	case "darwin":
		cmd = exec.Command("/bin/zsh", "-c", req.Command)
	default:
		cmd = exec.Command("/bin/bash", "-c", req.Command)
	}
	cmd.Dir = b.baseDir

	// Create a new process group so we can kill the entire tree.
	// (platform-specific; see exec_unix.go / exec_windows.go)
	setProcGroup(cmd)

	// Capture stdout+stderr via a buffer (instead of CombinedOutput) so we
	// can Start + Wait manually and kill the process group on cancellation.
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		exitCode := -1
		errMsg := fmt.Sprintf("failed to start command: %v", err)
		return &filesystem.ExecuteResponse{
			Output:   errMsg,
			ExitCode: &exitCode,
		}, nil
	}

	// Wait for the command in a separate goroutine.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	// Block until the command finishes, the timeout fires, or the caller cancels.
	var cmdErr error
	timedOut := false
	cancelled := false
	select {
	case cmdErr = <-waitDone:
		// Command finished normally (success or failure).
	case <-execCtx.Done():
		// Timeout or upstream cancellation — kill the entire process group.
		timedOut = execCtx.Err() == context.DeadlineExceeded
		cancelled = !timedOut
		killProcessGroup(cmd)
		// Wait for cmd.Wait to return so we can safely read the buffer.
		cmdErr = <-waitDone
	}

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	const maxOutput = 128 * 1024
	outputStr := buf.String()
	truncated := false
	if len(outputStr) > maxOutput {
		outputStr = outputStr[:maxOutput]
		truncated = true
	}

	if timedOut {
		outputStr += fmt.Sprintf("\n[Command timed out after %s]", timeout)
	} else if cancelled {
		outputStr += "\n[Command cancelled]"
	}

	if cmdErr != nil && len(outputStr) == 0 {
		outputStr = cmdErr.Error()
	}

	// Always include exit code — some LLM APIs reject empty tool result content.
	if outputStr == "" {
		outputStr = fmt.Sprintf("[exit code: %d]", exitCode)
	} else {
		outputStr = fmt.Sprintf("%s\n[exit code: %d]", outputStr, exitCode)
	}

	return &filesystem.ExecuteResponse{
		Output:    outputStr,
		ExitCode:  &exitCode,
		Truncated: truncated,
	}, nil
}

func (b *LocalBackend) validateCommand(command string) error {
	if b.policy == nil {
		return nil
	}
	for _, blocked := range b.policy.BlockedCommands {
		if strings.Contains(command, blocked) {
			return fmt.Errorf("command contains blocked pattern: %q", blocked)
		}
	}
	return nil
}
