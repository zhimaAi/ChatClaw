package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
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

var _ io.Writer = (*safeBuffer)(nil)

type bgProcess struct {
	cmd      *exec.Cmd
	pid      int
	command  string
	buf      *safeBuffer
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex
	exited   bool
	exitCode int
}

// BgProcessManager tracks background processes spawned by execute_background.
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
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
		}
	}
}

type bgExecInput struct {
	Command string `json:"command" jsonschema:"description=Shell command to run in the background (e.g. dev servers)."`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=Max seconds the background process may live before being auto-killed (default 300, max 600)."`
}

// NewBgExecuteTool creates the execute_background tool backed by Backend.
func NewBgExecuteTool(b *Backend, mgr *BgProcessManager) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDExecuteBackground,
		"Start a long-running command in the background (e.g. dev servers). Returns the pid and initial output. The process is auto-killed after timeout seconds (default 300, max 600). To stop or check status of the process, use the execute tool with action='stop' or action='status'.",
		func(ctx context.Context, input *bgExecInput) (string, error) {
			return bgStart(b, mgr, input)
		})
}

func bgStart(b *Backend, mgr *BgProcessManager, input *bgExecInput) (string, error) {
	if input.Command == "" {
		return "Error: command is required.", nil
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

	bgCtx, bgCancel := context.WithCancel(context.Background())

	var cmd *exec.Cmd
	if b.SandboxEnabled() {
		cmd = b.buildCodexCommand(input.Command)
	} else {
		cmd = buildNativeShellCommand(input.Command)
		cmd.Dir = b.WorkDir()
	}
	b.applyToolchainEnv(cmd)
	setProcGroup(cmd)

	pr, pw, pipeErr := os.Pipe()
	if pipeErr != nil {
		bgCancel()
		return fmt.Sprintf("Failed to create pipe: %v", pipeErr), nil
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		bgCancel()
		pw.Close()
		pr.Close()
		return fmt.Sprintf("Failed to start: %v", err), nil
	}
	pw.Close()

	buf := &safeBuffer{}
	go func() {
		_, _ = io.Copy(buf, pr)
	}()

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

	go func() {
		defer close(done)

		timer := time.NewTimer(time.Duration(timeoutSec) * time.Second)
		defer timer.Stop()

		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()

		select {
		case <-waitCh:
		case <-bgCtx.Done():
			killProcessGroup(cmd)
			pr.Close()
			select {
			case <-waitCh:
			case <-time.After(5 * time.Second):
			}
		case <-timer.C:
			killProcessGroup(cmd)
			pr.Close()
			select {
			case <-waitCh:
			case <-time.After(5 * time.Second):
			}
			_, _ = p.buf.Write([]byte(fmt.Sprintf("\n[Process auto-killed after %ds timeout]", timeoutSec)))
		}

		pr.Close()

		p.mu.Lock()
		p.exited = true
		if cmd.ProcessState != nil {
			p.exitCode = cmd.ProcessState.ExitCode()
		}
		p.mu.Unlock()
		mgr.remove(pid)
	}()

	select {
	case <-done:
		p.mu.Lock()
		output := truncateOutput(p.buf.String())
		code := p.exitCode
		p.mu.Unlock()
		return fmt.Sprintf("Process exited immediately.\n%s\n[exit code: %d]", output, code), nil
	case <-time.After(bgStartupWait):
		p.mu.Lock()
		output := truncateOutput(p.buf.String())
		p.mu.Unlock()
		return fmt.Sprintf("Background process started (pid=%d, auto-kill in %ds).\n%s", pid, timeoutSec, output), nil
	}
}

func truncateOutput(s string) string {
	if len(s) > maxBgOutput {
		return s[len(s)-maxBgOutput:]
	}
	return s
}
