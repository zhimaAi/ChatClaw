package filesystem

import (
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
	DefaultTimeout  time.Duration // Max execution time per command. 0 = 120s default.
}

// Execute runs a shell command and returns its output.
// Shell: powershell on Windows, zsh on macOS, bash on Linux.
// Working directory: baseDir.
func (b *LocalBackend) Execute(ctx context.Context, req *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	if err := b.validateCommand(req.Command); err != nil {
		exitCode := -1
		return &filesystem.ExecuteResponse{
			Output:   "Command blocked: " + err.Error(),
			ExitCode: &exitCode,
		}, nil
	}

	timeout := 120 * time.Second
	if b.policy != nil && b.policy.DefaultTimeout > 0 {
		timeout = b.policy.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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
		cmd = exec.CommandContext(ctx, "powershell.exe",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-Command", wrappedCmd,
		)
	case "darwin":
		cmd = exec.CommandContext(ctx, "/bin/zsh", "-c", req.Command)
	default:
		cmd = exec.CommandContext(ctx, "/bin/bash", "-c", req.Command)
	}
	cmd.Dir = b.baseDir

	output, err := cmd.CombinedOutput()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	const maxOutput = 128 * 1024
	outputStr := string(output)
	truncated := false
	if len(outputStr) > maxOutput {
		outputStr = outputStr[:maxOutput]
		truncated = true
	}

	if ctx.Err() == context.DeadlineExceeded {
		outputStr += "\n[Command timed out]"
	}

	if err != nil && len(outputStr) == 0 {
		outputStr = err.Error()
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
