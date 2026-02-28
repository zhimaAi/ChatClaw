package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const (
	defaultBgTimeout = 300
	maxBgTimeout     = 600
	bgStartupWait   = 5 * time.Second
	maxBgOutput      = 128 * 1024
)

// safeBuffer is a concurrency-safe byte buffer.
// cmd.Stdout/Stderr write to it from the OS thread while bgStatus reads
// from the user-facing goroutine — they must not race on the underlying slice.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Ensure safeBuffer satisfies io.Writer so it can be assigned to cmd.Stdout.
var _ io.Writer = (*safeBuffer)(nil)

type bgProcess struct {
	cmd      *exec.Cmd
	pid      int
	command  string
	buf      *safeBuffer
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex // protects exited / exitCode only
	exited   bool
	exitCode int
}

// BgProcessManager tracks background processes spawned by execute_background.
// Call Cleanup() when the agent session ends to kill all remaining processes.
type BgProcessManager struct {
	mu    sync.Mutex
	procs map[int]*bgProcess
}

func NewBgProcessManager() *BgProcessManager {
	return &BgProcessManager{procs: make(map[int]*bgProcess)}
}

func (m *BgProcessManager) add(p *bgProcess) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.procs[p.pid] = p
}

func (m *BgProcessManager) get(pid int) *bgProcess {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.procs[pid]
}

func (m *BgProcessManager) remove(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.procs, pid)
}

// Cleanup kills all remaining background processes.
func (m *BgProcessManager) Cleanup() {
	m.mu.Lock()
	snapshot := make([]*bgProcess, 0, len(m.procs))
	for _, p := range m.procs {
		snapshot = append(snapshot, p)
	}
	m.procs = make(map[int]*bgProcess)
	m.mu.Unlock()

	for _, p := range snapshot {
		p.cancel()
		<-p.done
	}
}

type bgExecInput struct {
	Action  string `json:"action" jsonschema:"description=Action to perform: 'start' to launch a background command, 'stop' to kill a running process, 'status' to check if a process is still alive and read its latest output.,enum=start,enum=stop,enum=status"`
	Command string `json:"command,omitempty" jsonschema:"description=Shell command to run (required for action=start)."`
	PID     int    `json:"pid,omitempty" jsonschema:"description=Process ID returned by a previous start (required for action=stop and action=status)."`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=Max seconds the background process may live before being auto-killed (default 300, max 600). Only used with action=start."`
}

// NewBgExecuteTool creates the execute_background tool.
func NewBgExecuteTool(cfg *FsToolsConfig, mgr *BgProcessManager) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDExecuteBackground,
		"Run a command in the background (e.g. dev servers), stop it, or check its status. Use action='start' to launch, action='stop' to kill, action='status' to read output. The process is auto-killed after timeout seconds.",
		func(ctx context.Context, input *bgExecInput) (string, error) {
			switch input.Action {
			case "start":
				return bgStart(cfg, mgr, input)
			case "stop":
				return bgStop(mgr, input.PID)
			case "status":
				return bgStatus(mgr, input.PID)
			default:
				return "Unknown action. Use 'start', 'stop', or 'status'.", nil
			}
		})
}

func bgStart(cfg *FsToolsConfig, mgr *BgProcessManager, input *bgExecInput) (string, error) {
	if input.Command == "" {
		return "Error: command is required for action=start.", nil
	}
	if err := validateCommand(input.Command); err != nil {
		return fmt.Sprintf("Command blocked: %s", err.Error()), nil
	}

	timeoutSec := input.Timeout
	if timeoutSec <= 0 {
		timeoutSec = defaultBgTimeout
	}
	if timeoutSec > maxBgTimeout {
		timeoutSec = maxBgTimeout
	}

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = cfg.HomeDir
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())

	var cmd *exec.Cmd
	if cfg.SandboxEnabled && cfg.CodexBin != "" {
		cmd = buildCodexCommand(cfg, workDir, input.Command)
	} else {
		cmd = buildNativeCommand(input.Command)
		cmd.Dir = workDir
	}

	setProcGroup(cmd)

	buf := &safeBuffer{}
	cmd.Stdout = buf
	cmd.Stderr = buf

	if err := cmd.Start(); err != nil {
		bgCancel()
		return fmt.Sprintf("Failed to start: %v", err), nil
	}

	pid := cmd.Process.Pid
	done := make(chan struct{})

	p := &bgProcess{
		cmd:     cmd,
		pid:     pid,
		command: input.Command,
		buf:     buf,
		cancel:  bgCancel,
		done:    done,
	}
	mgr.add(p)

	// Background goroutine: wait for process exit or cancellation.
	go func() {
		defer close(done)

		timer := time.NewTimer(time.Duration(timeoutSec) * time.Second)
		defer timer.Stop()

		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()

		select {
		case <-waitCh:
			// Process exited on its own
		case <-bgCtx.Done():
			killProcessGroup(cmd)
			<-waitCh
		case <-timer.C:
			killProcessGroup(cmd)
			<-waitCh
			_, _ = p.buf.Write([]byte(fmt.Sprintf("\n[Process auto-killed after %ds timeout]", timeoutSec)))
		}

		p.mu.Lock()
		p.exited = true
		if cmd.ProcessState != nil {
			p.exitCode = cmd.ProcessState.ExitCode()
		}
		p.mu.Unlock()
		mgr.remove(pid)
	}()

	// Wait a few seconds to collect startup output.
	select {
	case <-done:
		// Process already exited during startup wait
		p.mu.Lock()
		output := truncateOutput(p.buf.String())
		code := p.exitCode
		p.mu.Unlock()
		return fmt.Sprintf("Process exited immediately.\n%s\n[exit code: %d]", output, code), nil
	case <-time.After(bgStartupWait):
		// Still running — good, return initial output
		p.mu.Lock()
		output := truncateOutput(p.buf.String())
		p.mu.Unlock()
		return fmt.Sprintf("Background process started (pid=%d, auto-kill in %ds).\n%s", pid, timeoutSec, output), nil
	}
}

func bgStop(mgr *BgProcessManager, pid int) (string, error) {
	if pid <= 0 {
		return "Error: pid is required for action=stop.", nil
	}
	p := mgr.get(pid)
	if p == nil {
		return fmt.Sprintf("No background process found with pid=%d. It may have already exited.", pid), nil
	}

	p.cancel()
	<-p.done

	p.mu.Lock()
	output := truncateOutput(p.buf.String())
	code := p.exitCode
	p.mu.Unlock()

	return fmt.Sprintf("Process %d stopped.\n%s\n[exit code: %d]", pid, output, code), nil
}

func bgStatus(mgr *BgProcessManager, pid int) (string, error) {
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

func truncateOutput(s string) string {
	if len(s) > maxBgOutput {
		return s[len(s)-maxBgOutput:]
	}
	return s
}
