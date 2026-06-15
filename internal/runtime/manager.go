package runtime

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"vinollama/internal/config"
	"vinollama/internal/llamacpp"
	"vinollama/internal/models"
)

type Manager struct {
	cfg      config.Config
	store    models.Store
	resolver llamacpp.BinaryResolver
	waiter   llamacpp.ReadyWaiter
	checker  llamacpp.HealthChecker
	client   llamacpp.LlamaClient
	logDir   string

	mu        sync.Mutex
	processes map[string]*llamacpp.ProcessHandle
	starts    map[string]*startCall
}

type startCall struct {
	done   chan struct{}
	handle *llamacpp.ProcessHandle
	err    error
}

type ManagerOptions struct {
	Config   config.Config
	Store    models.Store
	Resolver llamacpp.BinaryResolver
	Waiter   llamacpp.ReadyWaiter
	Checker  llamacpp.HealthChecker
	Client   llamacpp.LlamaClient
	LogDir   string
}

type StartOptions struct {
	Backend     string
	ContextSize int
	Threads     int
	BatchSize   int
	ExtraArgs   []string
}

func NewManager(options ManagerOptions) (*Manager, error) {
	if options.Store.Dir == "" {
		modelDir, err := config.ModelsDirectory(options.Config)
		if err != nil {
			return nil, err
		}
		options.Store, err = models.NewStore(modelDir)
		if err != nil {
			return nil, err
		}
	}
	if options.Resolver == nil {
		options.Resolver = llamacpp.NewBinaryResolver()
	}
	if options.Waiter == nil {
		options.Waiter = llamacpp.NewReadyWaiter(options.Config.Runtime.HealthPath, options.Config.Runtime.ReadyTimeout)
	}
	if options.Checker == nil {
		checker := llamacpp.NewHTTPHealthChecker(options.Config.Runtime.HealthPath)
		options.Checker = checker
	}
	if options.Client == nil {
		client := llamacpp.NewHTTPClient()
		options.Client = client
	}
	if options.LogDir == "" {
		root, err := config.DefaultRootDir()
		if err == nil {
			options.LogDir = filepath.Join(root, "logs", "runtime")
		}
	}
	return &Manager{
		cfg:       options.Config,
		store:     options.Store,
		resolver:  options.Resolver,
		waiter:    options.Waiter,
		checker:   options.Checker,
		client:    options.Client,
		logDir:    options.LogDir,
		processes: map[string]*llamacpp.ProcessHandle{},
		starts:    map[string]*startCall{},
	}, nil
}

func (m *Manager) ProxyGenerate(ctx context.Context, req llamacpp.GenerateRequest) (*llamacpp.GenerateResponse, error) {
	handle, err := m.GetOrStartModel(ctx, req.Model, StartOptions{})
	if err != nil {
		return nil, err
	}
	handle.Touch()
	return m.client.Generate(ctx, m.BaseURL(handle), req)
}

func (m *Manager) ProxyGenerateStream(ctx context.Context, req llamacpp.GenerateRequest) (<-chan llamacpp.StreamChunk, error) {
	handle, err := m.GetOrStartModel(ctx, req.Model, StartOptions{})
	if err != nil {
		return nil, err
	}
	handle.Touch()
	return m.client.GenerateStream(ctx, m.BaseURL(handle), req)
}

func (m *Manager) ProxyChat(ctx context.Context, req llamacpp.ChatRequest) (*llamacpp.ChatResponse, error) {
	handle, err := m.GetOrStartModel(ctx, req.Model, StartOptions{})
	if err != nil {
		return nil, err
	}
	handle.Touch()
	return m.client.Chat(ctx, m.BaseURL(handle), req)
}

func (m *Manager) ProxyChatStream(ctx context.Context, req llamacpp.ChatRequest) (<-chan llamacpp.StreamChunk, error) {
	handle, err := m.GetOrStartModel(ctx, req.Model, StartOptions{})
	if err != nil {
		return nil, err
	}
	handle.Touch()
	return m.client.ChatStream(ctx, m.BaseURL(handle), req)
}

func (m *Manager) GetOrStartModel(ctx context.Context, modelName string, options StartOptions) (*llamacpp.ProcessHandle, error) {
	backend := strings.TrimSpace(options.Backend)
	if backend == "" {
		backend = m.cfg.Runtime.Backend
	}
	if backend == "" {
		backend = "auto"
	}
	if backend != "auto" && backend != "openvino" && backend != "cpu" {
		return nil, llamacpp.ActionableError{
			What:    "Failed to start llama.cpp backend.",
			Reason:  fmt.Sprintf("unsupported backend %q", backend),
			Fix:     "Use one of: auto, openvino, cpu.",
			Details: fmt.Sprintf("model=%s", modelName),
		}
	}

	if existing := m.findReusable(modelName, backend); existing != nil {
		existing.Touch()
		return existing, nil
	}

	callKey := modelName + "|" + backend
	m.mu.Lock()
	if call := m.starts[callKey]; call != nil {
		m.mu.Unlock()
		select {
		case <-call.done:
			return call.handle, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &startCall{done: make(chan struct{})}
	m.starts[callKey] = call
	m.mu.Unlock()

	handle, err := m.startModel(ctx, modelName, backend, options)
	call.handle = handle
	call.err = err
	close(call.done)

	m.mu.Lock()
	delete(m.starts, callKey)
	m.mu.Unlock()
	return handle, err
}

func (m *Manager) StopModel(ctx context.Context, modelName string) (bool, error) {
	m.mu.Lock()
	var handles []*llamacpp.ProcessHandle
	for _, handle := range m.processes {
		if handle.Snapshot().Model == modelName {
			handles = append(handles, handle)
		}
	}
	m.mu.Unlock()
	if len(handles) == 0 {
		return false, nil
	}
	var firstErr error
	for _, handle := range handles {
		if err := handle.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return true, firstErr
}

func (m *Manager) RestartModel(ctx context.Context, modelName string, options StartOptions) (*llamacpp.ProcessHandle, bool, error) {
	stopped, err := m.StopModel(ctx, modelName)
	if err != nil {
		return nil, stopped, err
	}
	handle, err := m.GetOrStartModel(ctx, modelName, options)
	return handle, stopped, err
}

func (m *Manager) ListProcesses() []llamacpp.ProcessHandle {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]llamacpp.ProcessHandle, 0, len(m.processes))
	for _, handle := range m.processes {
		out = append(out, handle.Snapshot())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Model == out[j].Model {
			return out[i].Backend < out[j].Backend
		}
		return out[i].Model < out[j].Model
	})
	return out
}

func (m *Manager) GetProcess(modelName, backend string) (llamacpp.ProcessHandle, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if handle, ok := m.processes[processKey(modelName, backend)]; ok {
		return handle.Snapshot(), true
	}
	return llamacpp.ProcessHandle{}, false
}

func (m *Manager) ShutdownIdle(ctx context.Context, now time.Time) (int, error) {
	timeout := m.cfg.Runtime.IdleTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	m.mu.Lock()
	var idle []*llamacpp.ProcessHandle
	for _, handle := range m.processes {
		snapshot := handle.Snapshot()
		if snapshot.State == llamacpp.ProcessReady && now.Sub(snapshot.LastUsedAt) >= timeout {
			idle = append(idle, handle)
		}
	}
	m.mu.Unlock()
	var firstErr error
	for _, handle := range idle {
		if err := handle.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return len(idle), firstErr
}

func (m *Manager) ShutdownAll(ctx context.Context) error {
	m.mu.Lock()
	handles := make([]*llamacpp.ProcessHandle, 0, len(m.processes))
	for _, handle := range m.processes {
		handles = append(handles, handle)
	}
	m.mu.Unlock()
	var firstErr error
	for _, handle := range handles {
		if err := handle.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Manager) BaseURL(handle *llamacpp.ProcessHandle) string {
	if handle == nil {
		return ""
	}
	u := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", handle.Host, handle.Port),
	}
	return u.String()
}

func (m *Manager) LogDir() string {
	if m == nil {
		return ""
	}
	return m.logDir
}

func (m *Manager) findReusable(modelName, backend string) *llamacpp.ProcessHandle {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, handle := range m.processes {
		snapshot := handle.Snapshot()
		if snapshot.Model != modelName {
			continue
		}
		if backend != "auto" && snapshot.Backend != backend {
			continue
		}
		if snapshot.State == llamacpp.ProcessReady {
			return handle
		}
	}
	return nil
}

func (m *Manager) startModel(ctx context.Context, modelName, backend string, options StartOptions) (*llamacpp.ProcessHandle, error) {
	manifest, err := m.store.ReadManifest(modelName)
	if err != nil {
		return nil, llamacpp.ActionableError{
			What:    "Failed to start llama.cpp backend.",
			Reason:  err.Error(),
			Fix:     "Import the GGUF model first with `vinollama import <name> <path-to-gguf>`.",
			Details: fmt.Sprintf("model=%s backend=%s", modelName, backend),
		}
	}
	actualBackend, binary, resolveErr := m.resolveBackend(ctx, backend)
	if resolveErr != nil {
		return nil, resolveErr
	}
	caps := llamacpp.DetectCapabilities(binary.HelpOutput)
	port, err := AllocatePort(m.cfg.Runtime.InternalPortStart)
	if err != nil {
		return nil, llamacpp.ActionableError{
			What:    "Failed to allocate llama.cpp internal port.",
			Reason:  err.Error(),
			Fix:     "Stop the process using the configured internal port range or change runtime.internal_port_start.",
			Details: fmt.Sprintf("model=%s backend=%s", modelName, actualBackend),
		}
	}
	extraArgs := append([]string{}, options.ExtraArgs...)
	switch actualBackend {
	case "openvino":
		extraArgs = append(extraArgs, m.cfg.Runtime.ExtraOpenVINOArgs...)
	case "cpu":
		extraArgs = append(extraArgs, m.cfg.Runtime.ExtraCPUArgs...)
	}
	contextSize := options.ContextSize
	if contextSize <= 0 {
		contextSize = m.cfg.Generation.CtxSize
	}
	threads := options.Threads
	if threads <= 0 {
		threads = m.cfg.Generation.Threads
	}
	cmd, err := llamacpp.BuildServerCommand(llamacpp.ServerStartRequest{
		BinaryPath:  binary.Path,
		ModelPath:   manifest.Path,
		Host:        "127.0.0.1",
		Port:        port,
		Backend:     actualBackend,
		ContextSize: contextSize,
		Threads:     threads,
		BatchSize:   options.BatchSize,
		Temperature: m.cfg.Generation.Temperature,
		TopP:        m.cfg.Generation.TopP,
		ExtraArgs:   extraArgs,
	}, caps, m.cfg.Runtime.AllowUnverifiedFlags)
	if err != nil {
		return nil, err
	}
	handle, err := llamacpp.StartManagedProcess(ctx, llamacpp.ManagedProcessStartRequest{
		ID:      processKey(manifest.Name, actualBackend),
		Model:   manifest.Name,
		Backend: actualBackend,
		Host:    "127.0.0.1",
		Port:    port,
		Command: cmd,
		LogDir:  m.logDir,
	})
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.processes[processKey(manifest.Name, actualBackend)] = handle
	m.mu.Unlock()

	baseURL := m.BaseURL(handle)
	if err := m.waiter.WaitReady(ctx, baseURL); err != nil {
		handle.SetFailed(err)
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = handle.Stop(stopCtx)
		cancel()
		return handle, llamacpp.ActionableError{
			What:    fmt.Sprintf("Failed to start llama.cpp %s backend.", actualBackend),
			Reason:  err.Error(),
			Fix:     "Run `vinollama doctor --model " + manifest.Name + "` and inspect the runtime log.",
			Details: fmt.Sprintf("backend=%s binary=%s model=%s port=%d stderr_tail=%q", actualBackend, binary.Path, manifest.Path, port, handle.StderrTail()),
		}
	}
	handle.MarkReady()
	return handle, nil
}

func (m *Manager) resolveBackend(ctx context.Context, backend string) (string, llamacpp.ResolvedBinary, error) {
	if backend == "openvino" {
		binary, err := m.resolver.Resolve(ctx, llamacpp.BinaryKindOpenVINO, m.cfg)
		return "openvino", binary, err
	}
	if backend == "cpu" {
		binary, err := m.resolver.Resolve(ctx, llamacpp.BinaryKindCPU, m.cfg)
		return "cpu", binary, err
	}
	openvino, err := m.resolver.Resolve(ctx, llamacpp.BinaryKindOpenVINO, m.cfg)
	if err == nil {
		return "openvino", openvino, nil
	}
	cpu, cpuErr := m.resolver.Resolve(ctx, llamacpp.BinaryKindCPU, m.cfg)
	if cpuErr == nil {
		return "cpu", cpu, nil
	}
	return "", llamacpp.ResolvedBinary{}, llamacpp.ActionableError{
		What:    "Auto backend has no available llama.cpp runtime.",
		Reason:  fmt.Sprintf("OpenVINO unavailable: %v; CPU unavailable: %v", err, cpuErr),
		Fix:     "Configure VINOLLAMA_LLAMA_OPENVINO_BIN or VINOLLAMA_LLAMA_CPU_BIN.",
		Details: "backend=auto",
	}
}

func processKey(modelName, backend string) string {
	return modelName + "|" + backend
}
