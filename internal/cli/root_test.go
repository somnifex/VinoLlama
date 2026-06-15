package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_HELP") == "1" && len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Print(`llama.cpp server version 1.2.3
  -m, --model FNAME
  --host HOST
  --port PORT
  -c, --ctx-size N
  -t, --threads N
`)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestHelpShowsSafeDefaults(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Execute(context.Background(), []string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Execute() code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"VinoLlama", "127.0.0.1", "11435", "Telemetry: false"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorRunsWithSafeConfig(t *testing.T) {
	dir := t.TempDir()
	modelDir := filepath.Join(dir, "models")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VINOLLAMA_FAKE_LLAMA_HELP", "1")
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("server:\n  host: 127.0.0.1\n  port: 11435\nruntime:\n  llama_cpu_bin: \""+exe+"\"\nmodels:\n  directory: "+modelDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--config", configPath, "doctor"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Execute() code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "VinoLlama doctor") {
		t.Fatalf("doctor output missing heading:\n%s", stdout.String())
	}
}

func TestImportListAndRemoveManifestOnly(t *testing.T) {
	dir := t.TempDir()
	modelDir := filepath.Join(dir, "models")
	source := filepath.Join(dir, "qwen2.5-7b-instruct-q4_k_m.gguf")
	if err := os.WriteFile(source, []byte("GGUF fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("models:\n  directory: "+modelDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--config", configPath, "import", "test-model", source, "--reference"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("import code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Imported test-model") {
		t.Fatalf("import output missing success:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute(context.Background(), []string{"--config", configPath, "list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{"NAME", "test-model", "7B", "Q4_K_M"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("list output missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute(context.Background(), []string{"--config", configPath, "rm", "test-model", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("rm code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source model should remain after manifest-only rm: %v", err)
	}
	if !strings.Contains(stdout.String(), "Model file left untouched.") {
		t.Fatalf("rm output should confirm model file was kept:\n%s", stdout.String())
	}
}
