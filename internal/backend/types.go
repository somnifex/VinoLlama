package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

const (
	LevelPass = "PASS"
	LevelWarn = "WARN"
	LevelFail = "FAIL"
)

type Backend interface {
	Name() string
	Check(ctx context.Context) DiagnosticResult
	Start(ctx context.Context, req StartRequest) (*ProcessHandle, error)
	Stop(ctx context.Context, handle *ProcessHandle) error
	Health(ctx context.Context, handle *ProcessHandle) error
}

type DiagnosticResult struct {
	Name   string
	Level  string
	What   string
	Reason string
	Fix    string
}

type StartRequest struct {
	ModelName         string
	ModelPath         string
	Host              string
	Port              int
	ContextSize       int
	Threads           int
	CommandPrefixArgs []string
	ExtraArgs         []string
	Env               []string
}

type ProcessHandle struct {
	ModelName   string
	BackendName string
	PID         int
	Port        int
	ContextSize int
	StartedAt   time.Time

	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{}

	mu      sync.Mutex
	waitErr error
}

func (h *ProcessHandle) Uptime(now time.Time) time.Duration {
	if h == nil || h.StartedAt.IsZero() {
		return 0
	}
	return now.Sub(h.StartedAt)
}

func (h *ProcessHandle) wait(ctx context.Context) error {
	if h == nil {
		return errors.New("process handle is nil")
	}
	select {
	case <-h.done:
		h.mu.Lock()
		err := h.waitErr
		h.mu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type LlamaCPPBackend struct {
	name   string
	binary string
}

func NewLlamaCPPBackend(name, binary string) *LlamaCPPBackend {
	return &LlamaCPPBackend{name: name, binary: binary}
}

func (b *LlamaCPPBackend) Name() string {
	return b.name
}

func (b *LlamaCPPBackend) Check(context.Context) DiagnosticResult {
	if b.binary == "" {
		return DiagnosticResult{
			Name:   b.name,
			Level:  LevelWarn,
			What:   "Backend binary is not configured.",
			Reason: fmt.Sprintf("No llama.cpp %s binary path is set.", b.name),
			Fix:    backendBinaryFix(b.name),
		}
	}
	info, err := os.Stat(b.binary)
	if err != nil {
		return DiagnosticResult{
			Name:   b.name,
			Level:  LevelWarn,
			What:   "Backend binary could not be accessed.",
			Reason: fmt.Sprintf("%s: %v", b.binary, err),
			Fix:    backendBinaryFix(b.name),
		}
	}
	if info.IsDir() {
		return DiagnosticResult{
			Name:   b.name,
			Level:  LevelFail,
			What:   "Backend binary path points to a directory.",
			Reason: b.binary,
			Fix:    backendBinaryFix(b.name),
		}
	}
	return DiagnosticResult{
		Name:   b.name,
		Level:  LevelPass,
		What:   "Backend binary is configured.",
		Reason: b.binary,
	}
}

func (b *LlamaCPPBackend) Start(ctx context.Context, req StartRequest) (*ProcessHandle, error) {
	if req.ModelPath == "" {
		return nil, ActionableError{
			What:   "Model process could not be started.",
			Reason: "StartRequest.ModelPath is empty.",
			Fix:    "Pass the path from a GGUF model manifest.",
		}
	}
	if req.Port <= 0 || req.Port > 65535 {
		return nil, ActionableError{
			What:   "Model process could not be started.",
			Reason: fmt.Sprintf("invalid internal port %d", req.Port),
			Fix:    "Choose a free internal port between 1 and 65535.",
		}
	}
	host := req.Host
	if host == "" {
		host = "127.0.0.1"
	}
	if host == "0.0.0.0" || host == "::" {
		return nil, ActionableError{
			What:   "Model process could not be started.",
			Reason: fmt.Sprintf("internal llama.cpp host %s would expose the process beyond localhost", host),
			Fix:    "Use 127.0.0.1 for internal llama.cpp processes.",
		}
	}
	req.Host = host
	if result := b.Check(ctx); result.Level != LevelPass {
		return nil, ActionableError{
			What:   fmt.Sprintf("%s backend is unavailable.", b.name),
			Reason: result.Reason,
			Fix:    result.Fix,
		}
	}

	processCtx, cancel := context.WithCancel(ctx)
	args := buildServerArgs(req)
	cmd := exec.CommandContext(processCtx, b.binary, args...)
	if len(req.Env) > 0 {
		cmd.Env = req.Env
	}
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, ActionableError{
			What:   fmt.Sprintf("%s backend process could not be started.", b.name),
			Reason: err.Error(),
			Fix:    "Check that the llama.cpp binary path is executable and compatible with this OS.",
		}
	}

	handle := &ProcessHandle{
		ModelName:   req.ModelName,
		BackendName: b.name,
		PID:         cmd.Process.Pid,
		Port:        req.Port,
		ContextSize: req.ContextSize,
		StartedAt:   time.Now().UTC(),
		cmd:         cmd,
		cancel:      cancel,
		done:        make(chan struct{}),
	}
	go func() {
		err := cmd.Wait()
		handle.mu.Lock()
		handle.waitErr = err
		handle.mu.Unlock()
		close(handle.done)
	}()
	return handle, nil
}

func (b *LlamaCPPBackend) Stop(ctx context.Context, handle *ProcessHandle) error {
	if handle == nil {
		return nil
	}
	select {
	case <-handle.done:
		return nil
	default:
	}
	if handle.cancel != nil {
		handle.cancel()
	}
	err := handle.wait(ctx)
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	return err
}

func (b *LlamaCPPBackend) Health(ctx context.Context, handle *ProcessHandle) error {
	if handle == nil {
		return errors.New("process handle is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-handle.done:
		handle.mu.Lock()
		err := handle.waitErr
		handle.mu.Unlock()
		if err != nil {
			return fmt.Errorf("%s backend process exited: %w", b.name, err)
		}
		return fmt.Errorf("%s backend process exited", b.name)
	default:
		return nil
	}
}

func buildServerArgs(req StartRequest) []string {
	host := req.Host
	if host == "" {
		host = "127.0.0.1"
	}
	args := append([]string{}, req.CommandPrefixArgs...)
	args = append(args, "-m", req.ModelPath, "--host", host, "--port", strconv.Itoa(req.Port))
	if req.ContextSize > 0 {
		args = append(args, "-c", strconv.Itoa(req.ContextSize))
	}
	if req.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(req.Threads))
	}
	args = append(args, req.ExtraArgs...)
	return args
}

func backendBinaryFix(name string) string {
	switch name {
	case "openvino":
		return "Set VINOLLAMA_LLAMA_OPENVINO_BIN or runtime.llama_openvino_bin."
	case "cpu":
		return "Set VINOLLAMA_LLAMA_CPU_BIN or runtime.llama_cpu_bin."
	default:
		return "Set the llama.cpp backend binary path in config.yaml or environment variables."
	}
}
