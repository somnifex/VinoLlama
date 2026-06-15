package llamacpp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"
)

type ProcessState string

const (
	ProcessStarting ProcessState = "starting"
	ProcessReady    ProcessState = "ready"
	ProcessFailed   ProcessState = "failed"
	ProcessStopping ProcessState = "stopping"
	ProcessStopped  ProcessState = "stopped"
)

type ProcessHandle struct {
	ID         string
	Model      string
	Backend    string
	PID        int
	Host       string
	Port       int
	State      ProcessState
	StartedAt  time.Time
	LastUsedAt time.Time
	Error      string
	LogPath    string

	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
	tail   *TailBuffer

	mu      sync.RWMutex
	waitErr error
}

type ManagedProcessStartRequest struct {
	ID      string
	Model   string
	Backend string
	Host    string
	Port    int
	Command ServerCommand
	LogDir  string
}

func StartManagedProcess(ctx context.Context, req ManagedProcessStartRequest) (*ProcessHandle, error) {
	if req.Command.Path == "" {
		return nil, ActionableError{
			What:    "Failed to start llama.cpp backend.",
			Reason:  "server command path is empty",
			Fix:     "Resolve the llama.cpp binary before starting a model.",
			Details: fmt.Sprintf("backend=%s model=%s port=%d", req.Backend, req.Model, req.Port),
		}
	}
	if req.ID == "" {
		req.ID = fmt.Sprintf("%s-%s-%d", req.Model, req.Backend, req.Port)
	}
	if req.Host == "" {
		req.Host = "127.0.0.1"
	}
	logPath, logWriter, closeLog, err := openProcessLog(req.LogDir, req.ID)
	if err != nil {
		return nil, err
	}

	processCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, req.Command.Path, req.Command.Args...)
	cmd.Env = append(os.Environ(), req.Command.Env...)
	tail := NewTailBuffer(8 * 1024)
	writer := io.MultiWriter(logWriter, tail)
	cmd.Stdout = writer
	cmd.Stderr = writer

	handle := &ProcessHandle{
		ID:         req.ID,
		Model:      req.Model,
		Backend:    req.Backend,
		Host:       req.Host,
		Port:       req.Port,
		State:      ProcessStarting,
		StartedAt:  time.Now().UTC(),
		LastUsedAt: time.Now().UTC(),
		LogPath:    logPath,
		cmd:        cmd,
		cancel:     cancel,
		done:       make(chan struct{}),
		tail:       tail,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		_ = closeLog()
		return nil, ActionableError{
			What:    fmt.Sprintf("Failed to start llama.cpp %s backend.", req.Backend),
			Reason:  err.Error(),
			Fix:     "Check that the llama.cpp binary path is executable and compatible with this OS.",
			Details: fmt.Sprintf("command=%s %v backend=%s model=%s port=%d stderr_tail=%q", req.Command.Path, req.Command.Args, req.Backend, req.Model, req.Port, stderrTail(tail.String())),
		}
	}
	handle.PID = cmd.Process.Pid

	go func() {
		err := cmd.Wait()
		_ = closeLog()
		handle.mu.Lock()
		handle.waitErr = err
		switch handle.State {
		case ProcessStopping:
			handle.State = ProcessStopped
		default:
			if err != nil {
				handle.State = ProcessFailed
				handle.Error = fmt.Sprintf("%v; stderr_tail=%s", err, stderrTail(tail.String()))
			} else {
				handle.State = ProcessStopped
			}
		}
		handle.mu.Unlock()
		close(handle.done)
	}()

	return handle, nil
}

func (h *ProcessHandle) MarkReady() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.State == ProcessStarting {
		h.State = ProcessReady
	}
	h.LastUsedAt = time.Now().UTC()
}

func (h *ProcessHandle) Touch() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.LastUsedAt = time.Now().UTC()
}

func (h *ProcessHandle) Stop(ctx context.Context) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	if h.State == ProcessStopped {
		h.mu.Unlock()
		return nil
	}
	if h.State != ProcessFailed {
		h.State = ProcessStopping
	}
	h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
	}

	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		if h.cmd != nil && h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
		}
		select {
		case <-h.done:
			return nil
		case <-time.After(2 * time.Second):
			return ctx.Err()
		}
	}
}

func (h *ProcessHandle) Done() <-chan struct{} {
	if h == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return h.done
}

func (h *ProcessHandle) WaitErr() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.waitErr
}

func (h *ProcessHandle) StderrTail() string {
	if h == nil || h.tail == nil {
		return ""
	}
	return stderrTail(h.tail.String())
}

func (h *ProcessHandle) Snapshot() ProcessHandle {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return ProcessHandle{
		ID:         h.ID,
		Model:      h.Model,
		Backend:    h.Backend,
		PID:        h.PID,
		Host:       h.Host,
		Port:       h.Port,
		State:      h.State,
		StartedAt:  h.StartedAt,
		LastUsedAt: h.LastUsedAt,
		Error:      h.Error,
		LogPath:    h.LogPath,
	}
}

func (h *ProcessHandle) SetFailed(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.State = ProcessFailed
	if err != nil {
		h.Error = err.Error()
	}
}

func openProcessLog(logDir, id string) (string, io.Writer, func() error, error) {
	if logDir == "" {
		return "", io.Discard, func() error { return nil }, nil
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", nil, nil, ActionableError{
			What:    "Failed to prepare llama.cpp runtime log directory.",
			Reason:  err.Error(),
			Fix:     "Set the VinoLlama root directory to a writable location or fix directory permissions.",
			Details: fmt.Sprintf("log_dir=%s", logDir),
		}
	}
	path := filepath.Join(logDir, sanitizeLogName(id)+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", nil, nil, ActionableError{
			What:    "Failed to open llama.cpp runtime log file.",
			Reason:  err.Error(),
			Fix:     "Check runtime log directory permissions.",
			Details: fmt.Sprintf("log_path=%s", path),
		}
	}
	return path, file, file.Close, nil
}

func sanitizeLogName(id string) string {
	re := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	name := re.ReplaceAllString(id, "_")
	if name == "" {
		return fmt.Sprintf("llama-%d", time.Now().UnixNano())
	}
	return name
}

func IsExpectedStopError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if runtime.GOOS == "windows" {
			return true
		}
		return true
	}
	return false
}
