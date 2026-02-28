package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// BlockedCommands are shell commands that are always rejected.
var BlockedCommands = []string{
	"rm -rf /", "rm -rf /*", "mkfs", "dd if=",
	":(){:|:&};:", "format c:", "format d:",
}

type executeInput struct {
	Command string `json:"command" jsonschema:"description=The shell command to execute."`
}

// NewExecuteTool creates an execute tool that runs shell commands.
// When codex sandbox is enabled, commands are wrapped with codex sandbox.
// Otherwise, commands run directly via the system shell.
func NewExecuteTool(cfg *FsToolsConfig) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDExecute,
		"Execute a shell command and return its output. Working directory is the configured workspace. Timeout: 60 seconds.",
		func(ctx context.Context, input *executeInput) (string, error) {
			if err := validateCommand(input.Command); err != nil {
				return fmt.Sprintf("Command blocked: %s", err.Error()), nil
			}

			timeout := 60 * time.Second
			execCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			workDir := cfg.WorkDir
			if workDir == "" {
				workDir = cfg.HomeDir
			}

			var cmd *exec.Cmd
			if cfg.SandboxEnabled && cfg.CodexBin != "" {
				cmd = buildCodexCommand(cfg.CodexBin, input.Command, workDir)
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

func buildCodexCommand(codexBin, command, workDir string) *exec.Cmd {
	platform := "macos"
	if runtime.GOOS == "linux" {
		platform = "linux"
	}

	args := []string{
		"sandbox", platform,
		"--full-auto",
		"--",
		"sh", "-c", command,
	}

	cmd := exec.Command(codexBin, args...)
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
