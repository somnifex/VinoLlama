package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_SERVER") == "1" {
		runCLIFakeLlamaServer()
		return
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

func TestServeHelpShowsCommandFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Execute(context.Background(), []string{"serve", "--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Execute() code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"vinollama serve", "--host", "--port", "--backend"} {
		if !strings.Contains(out, want) {
			t.Fatalf("serve help output missing %q:\n%s", want, out)
		}
	}
}

func TestParseServeArgsAcceptsDocumentedFlags(t *testing.T) {
	options, err := parseServeArgs([]string{"--host", "127.0.0.1", "--port", "12435", "--backend", "cpu", "--verbose"}, "", false)
	if err != nil {
		t.Fatalf("parseServeArgs returned error: %v", err)
	}
	if options.Host != "127.0.0.1" || options.Port != 12435 || options.Backend != "cpu" || !options.Verbose {
		t.Fatalf("options = %#v", options)
	}
}

func TestServeRejectsInvalidCommandFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Execute(context.Background(), []string{"serve", "--port", "not-a-port"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("Execute() code = %d, want 2; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Serve arguments are invalid.") {
		t.Fatalf("stderr missing actionable serve error:\n%s", stderr.String())
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

func TestRunChatsWithFakeLlamaServer(t *testing.T) {
	configPath, modelPath := writeCLIChatConfig(t)
	var stdout, stderr bytes.Buffer
	importCode := Execute(context.Background(), []string{"--config", configPath, "import", "test-model", modelPath, "--reference"}, &stdout, &stderr)
	if importCode != 0 {
		t.Fatalf("import code = %d, want 0; stdout:\n%s\nstderr:\n%s", importCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code := ExecuteWithIO(ctx, []string{"--config", configPath, "run", "test-model", "--backend", "cpu"}, strings.NewReader("hello\n/exit\n"), &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "fake cli chat") {
		t.Fatalf("run output missing fake response:\n%s", stdout.String())
	}
}

func TestRunStreamsWithFakeLlamaServer(t *testing.T) {
	configPath, modelPath := writeCLIChatConfig(t)
	var stdout, stderr bytes.Buffer
	importCode := Execute(context.Background(), []string{"--config", configPath, "import", "test-model", modelPath, "--reference"}, &stdout, &stderr)
	if importCode != 0 {
		t.Fatalf("import code = %d, want 0; stdout:\n%s\nstderr:\n%s", importCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code := ExecuteWithIO(ctx, []string{"--config", configPath, "run", "test-model", "--backend", "cpu", "--stream"}, strings.NewReader("hello\n/exit\n"), &stdout, &stderr)

	if code != 0 {
		t.Fatalf("stream run code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "fake streamed chat") {
		t.Fatalf("stream run output missing fake streamed response:\n%s", stdout.String())
	}
}

func TestRunFirstInterruptStopsGeneration(t *testing.T) {
	t.Setenv("VINOLLAMA_FAKE_CHAT_DELAY", "500ms")
	configPath, modelPath := writeCLIChatConfig(t)
	var stdout, stderr bytes.Buffer
	importCode := Execute(context.Background(), []string{"--config", configPath, "import", "test-model", modelPath, "--reference"}, &stdout, &stderr)
	if importCode != 0 {
		t.Fatalf("import code = %d, want 0; stdout:\n%s\nstderr:\n%s", importCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	interrupts := make(chan os.Signal, 2)
	go func() {
		time.Sleep(100 * time.Millisecond)
		interrupts <- os.Interrupt
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code := runInteractiveChatWithInterrupts(ctx, strings.NewReader("hello\n/exit\n"), &stdout, &stderr, configPath, "", []string{"test-model", "--backend", "cpu"}, interrupts)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Generation interrupted") {
		t.Fatalf("run output missing interruption notice:\n%s", stdout.String())
	}
}

func TestRunSecondInterruptExits(t *testing.T) {
	t.Setenv("VINOLLAMA_FAKE_CHAT_DELAY", "500ms")
	configPath, modelPath := writeCLIChatConfig(t)
	var stdout, stderr bytes.Buffer
	importCode := Execute(context.Background(), []string{"--config", configPath, "import", "test-model", modelPath, "--reference"}, &stdout, &stderr)
	if importCode != 0 {
		t.Fatalf("import code = %d, want 0; stdout:\n%s\nstderr:\n%s", importCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	interrupts := make(chan os.Signal, 2)
	go func() {
		time.Sleep(100 * time.Millisecond)
		interrupts <- os.Interrupt
		time.Sleep(100 * time.Millisecond)
		interrupts <- os.Interrupt
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	inputReader, inputWriter := io.Pipe()
	defer inputReader.Close()
	defer inputWriter.Close()
	go func() {
		_, _ = fmt.Fprintln(inputWriter, "hello")
	}()
	code := runInteractiveChatWithInterrupts(ctx, inputReader, &stdout, &stderr, configPath, "", []string{"test-model", "--backend", "cpu"}, interrupts)

	if code != 130 {
		t.Fatalf("run code = %d, want 130; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

func writeCLIChatConfig(t *testing.T) (string, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VINOLLAMA_FAKE_LLAMA_HELP", "1")
	t.Setenv("VINOLLAMA_FAKE_LLAMA_SERVER", "1")

	dir := t.TempDir()
	modelDir := filepath.Join(dir, "models")
	modelPath := filepath.Join(dir, "test-1b-q8_0.gguf")
	if err := os.WriteFile(modelPath, []byte("GGUF fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	configText := "server:\n  host: 127.0.0.1\n  port: 11435\nruntime:\n  backend: cpu\n  ready_timeout: 5s\n  llama_cpu_bin: " + yamlQuote(exe) + "\n  internal_port_start: " + strconv.Itoa(freeCLIPort(t)) + "\nmodels:\n  directory: " + yamlQuote(modelDir) + "\n"
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath, modelPath
}

func runCLIFakeLlamaServer() {
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
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "fail") {
			http.Error(w, "forced failure", http.StatusInternalServerError)
			return
		}
		var body bytes.Buffer
		_, _ = body.ReadFrom(r.Body)
		if delay, _ := time.ParseDuration(os.Getenv("VINOLLAMA_FAKE_CHAT_DELAY")); delay > 0 {
			time.Sleep(delay)
		}
		if strings.Contains(body.String(), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"fake "}}]}`)
			_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"streamed chat"}}]}`)
			_, _ = fmt.Fprintln(w, `data: [DONE]`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"choices":[{"message":{"role":"assistant","content":"fake cli chat"}}]}`)
	})
	if err := http.Serve(listener, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server stopped: %v\n", err)
	}
}

func freeCLIPort(t *testing.T) int {
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

func yamlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
