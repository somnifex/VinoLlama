package backend

import (
	"context"
	"strings"
	"testing"
)

type fakeBackend struct {
	name  string
	check DiagnosticResult
}

func (b fakeBackend) Name() string { return b.name }

func (b fakeBackend) Check(context.Context) DiagnosticResult { return b.check }

func (b fakeBackend) Start(context.Context, StartRequest) (*ProcessHandle, error) {
	return &ProcessHandle{BackendName: b.name}, nil
}

func (b fakeBackend) Stop(context.Context, *ProcessHandle) error { return nil }

func (b fakeBackend) Health(context.Context, *ProcessHandle) error { return nil }

func TestAutoBackendPrefersOpenVINO(t *testing.T) {
	auto := NewAutoBackend(
		fakeBackend{name: "openvino", check: DiagnosticResult{Level: LevelPass, Reason: "openvino ok"}},
		fakeBackend{name: "cpu", check: DiagnosticResult{Level: LevelPass, Reason: "cpu ok"}},
	)

	result := auto.Check(context.Background())
	if result.Level != LevelPass {
		t.Fatalf("Check() level = %s, want PASS", result.Level)
	}
	handle, err := auto.Start(context.Background(), StartRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if handle.BackendName != "openvino" {
		t.Fatalf("BackendName = %q, want openvino", handle.BackendName)
	}
}

func TestAutoBackendFallsBackToCPU(t *testing.T) {
	auto := NewAutoBackend(
		fakeBackend{name: "openvino", check: DiagnosticResult{Level: LevelWarn, Reason: "missing openvino"}},
		fakeBackend{name: "cpu", check: DiagnosticResult{Level: LevelPass, Reason: "cpu ok"}},
	)

	result := auto.Check(context.Background())
	if result.Level != LevelWarn {
		t.Fatalf("Check() level = %s, want WARN", result.Level)
	}
	handle, err := auto.Start(context.Background(), StartRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if handle.BackendName != "cpu" {
		t.Fatalf("BackendName = %q, want cpu", handle.BackendName)
	}
}

func TestAutoBackendFailsWhenNoRuntimeAvailable(t *testing.T) {
	auto := NewAutoBackend(
		fakeBackend{name: "openvino", check: DiagnosticResult{Level: LevelWarn, Reason: "missing openvino"}},
		fakeBackend{name: "cpu", check: DiagnosticResult{Level: LevelWarn, Reason: "missing cpu"}},
	)

	result := auto.Check(context.Background())
	if result.Level != LevelFail {
		t.Fatalf("Check() level = %s, want FAIL", result.Level)
	}
	if _, err := auto.Start(context.Background(), StartRequest{}); err == nil {
		t.Fatal("expected start to fail without runtime")
	}
}

func TestBuildServerArgsUseLlamaCPPConventions(t *testing.T) {
	args := buildServerArgs(StartRequest{
		ModelPath:   "model.gguf",
		Port:        21435,
		ContextSize: 512,
		Threads:     4,
	})

	got := strings.Join(args, " ")
	for _, want := range []string{"-m model.gguf", "--host 127.0.0.1", "--port 21435", "-c 512", "-t 4"} {
		if !strings.Contains(got, want) {
			t.Fatalf("args %q missing %q", got, want)
		}
	}
}

func TestStartRejectsPublicBind(t *testing.T) {
	b := NewLlamaCPPBackend("cpu", "unused")
	_, err := b.Start(context.Background(), StartRequest{
		ModelPath: "model.gguf",
		Host:      "0.0.0.0",
		Port:      21435,
	})
	if err == nil {
		t.Fatal("expected public bind rejection")
	}
}
