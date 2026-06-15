package cpu

import (
	"context"
	"os"
	"testing"
	"time"

	"vinollama/internal/backend"
)

func TestCPUBackendCheckWarnsWithoutBinary(t *testing.T) {
	result := New("").Check(context.Background())
	if result.Level != backend.LevelWarn {
		t.Fatalf("Check() level = %s, want WARN", result.Level)
	}
}

func TestCPUBackendStartHealthStop(t *testing.T) {
	if os.Getenv("VINOLLAMA_HELPER_PROCESS") == "1" {
		time.Sleep(5 * time.Minute)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b := New(exe)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	handle, err := b.Start(ctx, backend.StartRequest{
		ModelName:         "test-model",
		ModelPath:         "testdata/model.gguf",
		Host:              "127.0.0.1",
		Port:              21435,
		ContextSize:       128,
		CommandPrefixArgs: []string{"-test.run=TestCPUBackendStartHealthStop", "--"},
		Env:               append(os.Environ(), "VINOLLAMA_HELPER_PROCESS=1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if handle.PID == 0 {
		t.Fatal("expected a process PID")
	}
	if err := b.Health(ctx, handle); err != nil {
		t.Fatalf("Health() before stop = %v, want nil", err)
	}
	if err := b.Stop(ctx, handle); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}
}
