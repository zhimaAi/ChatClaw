package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// writableRoots lists home-relative directories that common package managers
// need write access to. These are added to the codex sandbox whitelist via
// sandbox_workspace_write.writable_roots so that tools like npm, bun, pip,
// yarn, etc. can use their caches without hitting "Operation not permitted".
var writableRoots = []string{
	".npm",
	".bun",
	".cache",
	".local",
	".yarn",
}

// BlockedCommands are shell commands that are always rejected.
var BlockedCommands = []string{
	"rm -rf /", "rm -rf /*", "mkfs", "dd if=",
	":(){:|:&};:", "format c:", "format d:",
}

type executeInput struct {
	Command string `json:"command" jsonschema:"description=The shell command to execute."`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=Maximum seconds to wait (default 60, max 300). Set higher for slow commands like npm install."`
}

const (
	defaultExecTimeout = 60
	maxExecTimeout     = 300
)

// NewExecuteTool creates an execute tool that runs shell commands synchronously.
// When codex sandbox is enabled, commands are wrapped with codex sandbox.
// Otherwise, commands run directly via the system shell.
func NewExecuteTool(cfg *FsToolsConfig) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDExecute,
		"Execute a shell command synchronously and return its output. Working directory is the configured workspace. Default timeout: 60s, max 300s. For long-running servers (npm run dev, etc.), use execute_background instead.",
		func(ctx context.Context, input *executeInput) (string, error) {
			if err := validateCommand(input.Command); err != nil {
				return fmt.Sprintf("Command blocked: %s", err.Error()), nil
			}

			timeoutSec := input.Timeout
			if timeoutSec <= 0 {
				timeoutSec = defaultExecTimeout
			}
			if timeoutSec > maxExecTimeout {
				timeoutSec = maxExecTimeout
			}
			timeout := time.Duration(timeoutSec) * time.Second
			execCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			workDir := cfg.WorkDir
			if workDir == "" {
				workDir = cfg.HomeDir
			}

			var cmd *exec.Cmd
			if cfg.SandboxEnabled && cfg.CodexBin != "" {
				cmd = buildCodexCommand(cfg, workDir, input.Command)
			} else {
				cmd = buildNativeCommand(input.Command)
				cmd.Dir = workDir
			}

			setProcGroup(cmd)

			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf

			if err := cmd.Start(); err != nil {
				return fmt.Sprintf("failed to start command: %v\n[exit code: -1]", err), nil
			}

			waitDone := make(chan error, 1)
			go func() { waitDone <- cmd.Wait() }()

			var cmdErr error
			timedOut := false
			cancelled := false
			select {
			case cmdErr = <-waitDone:
			case <-execCtx.Done():
				timedOut = execCtx.Err() == context.DeadlineExceeded
				cancelled = !timedOut
				killProcessGroup(cmd)
				cmdErr = <-waitDone
			}

			exitCode := 0
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}

			const maxOutput = 128 * 1024
			outputStr := buf.String()
			if len(outputStr) > maxOutput {
				outputStr = outputStr[:maxOutput]
			}

			if timedOut {
				outputStr += fmt.Sprintf("\n[Command timed out after %s]", timeout)
			} else if cancelled {
				outputStr += "\n[Command cancelled]"
			}

			if cmdErr != nil && len(outputStr) == 0 {
				outputStr = cmdErr.Error()
			}

			if outputStr == "" {
				outputStr = fmt.Sprintf("[exit code: %d]", exitCode)
			} else {
				outputStr = fmt.Sprintf("%s\n[exit code: %d]", outputStr, exitCode)
			}

			return outputStr, nil
		})
}

func validateCommand(command string) error {
	for _, blocked := range BlockedCommands {
		if strings.Contains(command, blocked) {
			return fmt.Errorf("command contains blocked pattern: %q", blocked)
		}
	}
	return nil
}

func buildCodexCommand(cfg *FsToolsConfig, workDir, command string) *exec.Cmd {
	platform := "macos"
	switch runtime.GOOS {
	case "linux":
		platform = "linux"
	case "windows":
		platform = "windows"
	}

	args := []string{"sandbox", platform, "--full-auto"}

	if cfg.SandboxNetworkEnabled {
		args = append(args, "-c", "sandbox_workspace_write.network_access=true")
	}

	roots := make([]string, 0, len(writableRoots))
	for _, rel := range writableRoots {
		roots = append(roots, fmt.Sprintf("%q", filepath.Join(cfg.HomeDir, rel)))
	}
	args = append(args, "-c",
		fmt.Sprintf("sandbox_workspace_write.writable_roots=[%s]", strings.Join(roots, ",")))

	if runtime.GOOS == "windows" {
		wrappedCmd := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
			"$OutputEncoding = [System.Text.Encoding]::UTF8; " +
			command
		args = append(args,
			"--",
			"powershell.exe",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-Command", wrappedCmd,
		)
	} else {
		args = append(args, "--", "sh", "-c", command)
	}

	cmd := exec.Command(cfg.CodexBin, args...)
	cmd.Dir = workDir
	return cmd
}

func buildNativeCommand(command string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		wrappedCmd := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
			"$OutputEncoding = [System.Text.Encoding]::UTF8; " +
			command
		return exec.Command("powershell.exe",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-Command", wrappedCmd,
		)
	case "darwin":
		return exec.Command("/bin/zsh", "-l", "-c", command)
	default:
		return exec.Command("/bin/bash", "-l", "-c", command)
	}
}
