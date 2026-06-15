package llamacpp

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestManagedProcessReadyHealthStopAndLog(t *testing.T) {
	exe := fakeExecutable(t)
	port := freePort(t)
	caps := DetectCapabilities(fakeHelp)
	cmd, err := BuildServerCommand(ServerStartRequest{
		BinaryPath: exe,
		ModelPath:  "model.gguf",
		Host:       "127.0.0.1",
		Port:       port,
		Backend:    "cpu",
		Env:        []string{"VINOLLAMA_FAKE_LLAMA_SERVER=1", "VINOLLAMA_FAKE_READY_DELAY=150ms"},
	}, caps, false)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	handle, err := StartManagedProcess(ctx, ManagedProcessStartRequest{
		ID:      "test-model-cpu",
		Model:   "test-model",
		Backend: "cpu",
		Host:    "127.0.0.1",
		Port:    port,
		Command: cmd,
		LogDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	baseURL := fmt.Sprintf("http://%s:%d", handle.Host, handle.Port)
	waiter := NewReadyWaiter("/health", 5*time.Second)
	if err := waiter.WaitReady(ctx, baseURL); err != nil {
		_ = handle.Stop(context.Background())
		t.Fatal(err)
	}
	handle.MarkReady()
	if state := handle.Snapshot().State; state != ProcessReady {
		t.Fatalf("State = %s, want ready", state)
	}
	if err := NewHTTPHealthChecker("/health").Check(ctx, baseURL); err != nil {
		t.Fatalf("health check = %v, want nil", err)
	}
	if _, err := os.Stat(handle.LogPath); err != nil {
		t.Fatalf("expected log file at %s: %v", handle.LogPath, err)
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	if err := handle.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}
	if state := handle.Snapshot().State; state != ProcessStopped {
		t.Fatalf("State after stop = %s, want stopped", state)
	}
}

func TestManagedProcessRecordsFailedExit(t *testing.T) {
	exe := fakeExecutable(t)
	caps := DetectCapabilities(fakeHelp)
	cmd, err := BuildServerCommand(ServerStartRequest{
		BinaryPath: exe,
		ModelPath:  "model.gguf",
		Host:       "127.0.0.1",
		Port:       freePort(t),
		Backend:    "cpu",
		Env:        []string{"VINOLLAMA_FAKE_LLAMA_EXIT=1"},
	}, caps, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	handle, err := StartManagedProcess(ctx, ManagedProcessStartRequest{
		ID:      "failed-model-cpu",
		Model:   "failed-model",
		Backend: "cpu",
		Host:    "127.0.0.1",
		Port:    freePort(t),
		Command: cmd,
		LogDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-handle.Done():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	snapshot := handle.Snapshot()
	if snapshot.State != ProcessFailed {
		t.Fatalf("State = %s, want failed", snapshot.State)
	}
	if snapshot.Error == "" || handle.StderrTail() == "" {
		t.Fatalf("expected failed process error and stderr tail, got error=%q tail=%q", snapshot.Error, handle.StderrTail())
	}
}

func freePort(t *testing.T) int {
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
