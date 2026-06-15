package runtime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"vinollama/internal/config"
	"vinollama/internal/llamacpp"
	"vinollama/internal/models"
)

const fakeRuntimeHelp = `llama.cpp server version 1.2.3
  -m, --model FNAME
  --host HOST
  --port PORT
  -c, --ctx-size N
  -t, --threads N
`

func TestMain(m *testing.M) {
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_HELP") == "1" && len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Print(fakeRuntimeHelp)
		os.Exit(0)
	}
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_EXIT") == "1" {
		fmt.Fprintln(os.Stderr, "fake runtime startup failure")
		os.Exit(9)
	}
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_SERVER") == "1" {
		runRuntimeFakeServer()
		return
	}
	os.Exit(m.Run())
}

func TestAllocatePortSkipsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	start, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	port, err := AllocatePort(start)
	if err != nil {
		t.Fatal(err)
	}
	if port == start {
		t.Fatalf("AllocatePort(%d) = occupied port %d", start, port)
	}
}

func TestRuntimeManagerReusesSameModelProcess(t *testing.T) {
	manager := newFakeManager(t, "cpu")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first, err := manager.GetOrStartModel(ctx, "test-model", StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.GetOrStartModel(ctx, "test-model", StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.PID != second.PID {
		t.Fatalf("PID = %d then %d, want reuse", first.PID, second.PID)
	}
	_ = manager.ShutdownAll(context.Background())
}

func TestRuntimeManagerStopModelUpdatesState(t *testing.T) {
	manager := newFakeManager(t, "cpu")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	handle, err := manager.GetOrStartModel(ctx, "test-model", StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := manager.StopModel(ctx, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("StopModel returned stopped=false")
	}
	if state := handle.Snapshot().State; state != llamacpp.ProcessStopped {
		t.Fatalf("State = %s, want stopped", state)
	}
}

func TestRuntimeManagerShutdownIdle(t *testing.T) {
	manager := newFakeManager(t, "cpu")
	manager.cfg.Runtime.IdleTimeout = time.Nanosecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	handle, err := manager.GetOrStartModel(ctx, "test-model", StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	count, err := manager.ShutdownIdle(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("ShutdownIdle stopped %d process(es), want 1", count)
	}
	if state := handle.Snapshot().State; state != llamacpp.ProcessStopped {
		t.Fatalf("State = %s, want stopped", state)
	}
}

func TestRuntimeManagerKeepsFailedStartupState(t *testing.T) {
	manager := newFakeManager(t, "cpu")
	t.Setenv("VINOLLAMA_FAKE_LLAMA_SERVER", "")
	t.Setenv("VINOLLAMA_FAKE_LLAMA_EXIT", "1")
	manager.cfg.Runtime.ReadyTimeout = 100 * time.Millisecond
	manager.waiter = llamacpp.NewReadyWaiter("/health", 100*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := manager.GetOrStartModel(ctx, "test-model", StartOptions{})
	if err == nil {
		t.Fatal("expected startup failure")
	}
	processes := manager.ListProcesses()
	if len(processes) != 1 {
		t.Fatalf("processes = %#v, want one failed process", processes)
	}
	if processes[0].State != llamacpp.ProcessFailed {
		t.Fatalf("State = %s, want failed", processes[0].State)
	}
}

func newFakeManager(t *testing.T, backend string) *Manager {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VINOLLAMA_FAKE_LLAMA_HELP", "1")
	t.Setenv("VINOLLAMA_FAKE_LLAMA_SERVER", "1")
	cfg := config.Defaults()
	cfg.Runtime.Backend = backend
	cfg.Runtime.ReadyTimeout = 5 * time.Second
	cfg.Runtime.InternalPortStart = freeRuntimePort(t)
	cfg.Models.Directory = filepath.Join(t.TempDir(), "models")
	store, err := models.NewStore(cfg.Models.Directory)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "test-1b-q8_0.gguf")
	if err := os.WriteFile(source, []byte("GGUF fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(models.ImportRequest{Name: "test-model", Path: source, Mode: models.SourceReference}); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(ManagerOptions{
		Config: cfg,
		Store:  store,
		Resolver: llamacpp.NewBinaryResolver(
			llamacpp.WithCLIBinary(llamacpp.BinaryKindCPU, exe),
			llamacpp.WithBundledRoot(t.TempDir()),
		),
		LogDir: filepath.Join(t.TempDir(), "logs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func runRuntimeFakeServer() {
	host := "127.0.0.1"
	port := 0
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--host":
			if i+1 < len(os.Args) {
				i++
				host = os.Args[i]
			}
		case "--port":
			if i+1 < len(os.Args) {
				i++
				port, _ = strconv.Atoi(os.Args[i])
			}
		}
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen failed: %v\n", err)
		os.Exit(3)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	if err := http.Serve(listener, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server stopped: %v\n", err)
	}
}

func freeRuntimePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return port
}
