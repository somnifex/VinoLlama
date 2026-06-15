package llamacpp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"vinollama/internal/config"
)

const fakeHelp = `llama.cpp server version 1.2.3
  -m, --model FNAME
  --host HOST
  --port PORT
  -c, --ctx-size N
  -t, --threads N
  -b, --batch-size N
  --device DEVICE
  --temp N
`

func TestMain(m *testing.M) {
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_HELP") == "1" && len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Print(fakeHelp)
		os.Exit(0)
	}
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_EXIT") == "1" {
		fmt.Fprintln(os.Stderr, "fake llama.cpp startup failure")
		os.Exit(7)
	}
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_SERVER") == "1" {
		runFakeLlamaServer()
		return
	}
	os.Exit(m.Run())
}

func TestBinaryResolverUsesCLIPathFirst(t *testing.T) {
	exe := fakeExecutable(t)
	t.Setenv("VINOLLAMA_FAKE_LLAMA_HELP", "1")
	t.Setenv("VINOLLAMA_LLAMA_CPU_BIN", filepath.Join(t.TempDir(), executableName("env-missing")))
	cfg := config.Defaults()
	cfg.Runtime.LlamaCPUBin = filepath.Join(t.TempDir(), executableName("config-missing"))

	resolved, err := NewBinaryResolver(WithCLIBinary(BinaryKindCPU, exe)).Resolve(context.Background(), BinaryKindCPU, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != "cli" {
		t.Fatalf("Source = %q, want cli", resolved.Source)
	}
	if resolved.Version != "1.2.3" {
		t.Fatalf("Version = %q, want 1.2.3", resolved.Version)
	}
}

func TestBinaryResolverUsesEnvBeforeConfig(t *testing.T) {
	exe := fakeExecutable(t)
	t.Setenv("VINOLLAMA_FAKE_LLAMA_HELP", "1")
	t.Setenv("VINOLLAMA_LLAMA_CPU_BIN", exe)
	cfg := config.Defaults()
	cfg.Runtime.LlamaCPUBin = filepath.Join(t.TempDir(), executableName("config-missing"))

	resolved, err := NewBinaryResolver().Resolve(context.Background(), BinaryKindCPU, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != "env" {
		t.Fatalf("Source = %q, want env", resolved.Source)
	}
}

func TestBinaryResolverUsesConfigPath(t *testing.T) {
	exe := fakeExecutable(t)
	t.Setenv("VINOLLAMA_FAKE_LLAMA_HELP", "1")
	cfg := config.Defaults()
	cfg.Runtime.LlamaCPUBin = exe

	resolved, err := NewBinaryResolver().Resolve(context.Background(), BinaryKindCPU, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != "config" {
		t.Fatalf("Source = %q, want config", resolved.Source)
	}
	if !strings.Contains(resolved.HelpOutput, "llama.cpp server") {
		t.Fatalf("HelpOutput = %q, want fake llama help", resolved.HelpOutput)
	}
}

func TestBinaryResolverUsesPATHLookup(t *testing.T) {
	exe := fakeExecutable(t)
	t.Setenv("VINOLLAMA_FAKE_LLAMA_HELP", "1")
	cfg := config.Defaults()
	resolver := NewBinaryResolver(WithBundledRoot(t.TempDir()), WithLookPath(func(name string) (string, error) {
		if strings.HasPrefix(name, "llama-server") {
			return exe, nil
		}
		return "", os.ErrNotExist
	}))

	resolved, err := resolver.Resolve(context.Background(), BinaryKindCPU, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != "path" {
		t.Fatalf("Source = %q, want path", resolved.Source)
	}
}

func TestBinaryResolverReportsMissingBinary(t *testing.T) {
	cfg := config.Defaults()
	cfg.Runtime.LlamaCPUBin = filepath.Join(t.TempDir(), executableName("missing"))

	_, err := NewBinaryResolver(WithBundledRoot(t.TempDir())).Resolve(context.Background(), BinaryKindCPU, cfg)
	if err == nil {
		t.Fatal("expected missing binary error")
	}
	if !strings.Contains(err.Error(), "could not be resolved") {
		t.Fatalf("error = %v, want resolution failure", err)
	}
}

func TestBinaryResolverRejectsNonExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-executable.txt")
	if err := os.WriteFile(path, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Runtime.LlamaCPUBin = path

	_, err := NewBinaryResolver(WithBundledRoot(t.TempDir())).Resolve(context.Background(), BinaryKindCPU, cfg)
	if err == nil {
		t.Fatal("expected non-executable error")
	}
	if !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("error = %v, want executable failure", err)
	}
}

func fakeExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func executableName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func runFakeLlamaServer() {
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
	if port == 0 {
		fmt.Fprintln(os.Stderr, "missing --port")
		os.Exit(2)
	}
	delay, _ := time.ParseDuration(os.Getenv("VINOLLAMA_FAKE_READY_DELAY"))
	readyAt := time.Now().Add(delay)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if time.Now().Before(readyAt) {
			http.Error(w, "warming up", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/completion", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"fake completion","stop":true}`))
	})
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen failed: %v\n", err)
		os.Exit(3)
	}
	fmt.Fprintf(os.Stderr, "fake llama server listening on %s\n", listener.Addr().String())
	if err := http.Serve(listener, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server stopped: %v\n", err)
	}
}
