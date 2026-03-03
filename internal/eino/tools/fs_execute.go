package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	Action  string `json:"action,omitempty" jsonschema:"description=Action to perform: 'run' (default) to execute a shell command, 'stop' to synchronously kill a background process by pid and wait for it to exit, 'status' to check if a background process is still alive and read its latest output.,enum=run,enum=stop,enum=status"`
	Command string `json:"command,omitempty" jsonschema:"description=The shell command to execute (required for action=run)."`
	PID     int    `json:"pid,omitempty" jsonschema:"description=Process ID returned by execute_background (required for action=stop and action=status)."`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=Maximum seconds to wait (default 30, max 300). For action=run: how long the command may run. For action=stop: how long to wait for the process to exit after sending kill signal."`
}

const (
	defaultExecTimeout = 30
	maxExecTimeout     = 300
	defaultStopTimeout = 10
)

// NewExecuteTool creates an execute tool backed by Backend.
func NewExecuteTool(b *Backend, bgMgr *BgProcessManager) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDExecute,
		"Execute a shell command synchronously (action='run', default), stop a background process (action='stop'), or check background process status (action='status'). Working directory is the configured workspace. Default timeout: 30s, max 300s. For long-running servers, use execute_background to start them, then use this tool with action='stop' or action='status' to manage them.",
		func(ctx context.Context, input *executeInput) (string, error) {
			action := input.Action
			if action == "" {
				action = "run"
			}
			switch action {
			case "run":
				return execRun(ctx, b, input)
			case "stop":
				return execStop(bgMgr, input)
			case "status":
				return execStatus(bgMgr, input.PID)
			default:
				return "Unknown action. Use 'run', 'stop', or 'status'.", nil
			}
		})
}

func execRun(ctx context.Context, b *Backend, input *executeInput) (string, error) {
	if input.Command == "" {
		return "Error: command is required for action=run.", nil
	}
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

	var cmd *exec.Cmd
	if b.SandboxEnabled() {
		cmd = b.buildCodexCommand(input.Command)
	} else {
		cmd = buildNativeShellCommand(input.Command)
		cmd.Dir = b.WorkDir()
	}
	b.applyToolchainEnv(cmd)
	setProcGroup(cmd)

	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Sprintf("failed to create pipe: %v\n[exit code: -1]", err), nil
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return fmt.Sprintf("failed to start command: %v\n[exit code: -1]", err), nil
	}
	pw.Close()

	var buf bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, pr)
		close(readDone)
	}()

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
		pr.Close()
		select {
		case cmdErr = <-waitDone:
		case <-time.After(5 * time.Second):
		}
	}

	pr.Close()
	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
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
}

func execStop(mgr *BgProcessManager, input *executeInput) (string, error) {
	if input.PID <= 0 {
		return "Error: pid is required for action=stop.", nil
	}
	p := mgr.get(input.PID)
	if p == nil {
		return fmt.Sprintf("No background process found with pid=%d. It may have already exited.", input.PID), nil
	}

	timeoutSec := input.Timeout
	if timeoutSec <= 0 {
		timeoutSec = defaultStopTimeout
	}
	if timeoutSec > maxExecTimeout {
		timeoutSec = maxExecTimeout
	}

	p.cancel()

	select {
	case <-p.done:
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		return fmt.Sprintf("Error: process %d did not exit within %ds after kill signal. The process may be stuck. Try 'kill -9 %d' via action=run, or investigate what is preventing it from exiting.", input.PID, timeoutSec, input.PID), nil
	}

	p.mu.Lock()
	output := truncateOutput(p.buf.String())
	code := p.exitCode
	p.mu.Unlock()

	return fmt.Sprintf("Process %d stopped successfully.\n%s\n[exit code: %d]", input.PID, output, code), nil
}

func execStatus(mgr *BgProcessManager, pid int) (string, error) {
	if pid <= 0 {
		return "Error: pid is required for action=status.", nil
	}
	p := mgr.get(pid)
	if p == nil {
		return fmt.Sprintf("No background process found with pid=%d. It may have already exited.", pid), nil
	}

	p.mu.Lock()
	output := truncateOutput(p.buf.String())
	exited := p.exited
	code := p.exitCode
	p.mu.Unlock()

	if exited {
		return fmt.Sprintf("Process %d has exited.\n%s\n[exit code: %d]", pid, output, code), nil
	}
	return fmt.Sprintf("Process %d is running.\nLatest output:\n%s", pid, output), nil
}

func validateCommand(command string) error {
	for _, blocked := range BlockedCommands {
		if strings.Contains(command, blocked) {
			return fmt.Errorf("command contains blocked pattern: %q", blocked)
		}
	}
	return nil
}
