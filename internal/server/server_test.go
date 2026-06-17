package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"vinollama/internal/config"
	"vinollama/internal/llamacpp"
	"vinollama/internal/models"
	vinoruntime "vinollama/internal/runtime"
)

const fakeServerHelp = `llama.cpp server version 1.2.3
  -m, --model FNAME
  --host HOST
  --port PORT
  -c, --ctx-size N
  -t, --threads N
`

func TestMain(m *testing.M) {
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_HELP") == "1" && len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Print(fakeServerHelp)
		os.Exit(0)
	}
	if os.Getenv("VINOLLAMA_FAKE_LLAMA_SERVER") == "1" {
		runAPIFakeLlamaServer()
		return
	}
	os.Exit(m.Run())
}

func TestAPIGenerateProxiesToFakeLlamaServer(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())
	api := httptest.NewServer(handler)
	defer api.Close()

	body := bytes.NewBufferString(`{"model":"test-model","prompt":"hello","stream":false}`)
	resp, err := http.Post(api.URL+"/api/generate", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var payload llamacpp.GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Response != "fake completion" || !payload.Done {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestAPIGenerateStreamsNDJSON(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())
	api := httptest.NewServer(handler)
	defer api.Close()

	body := bytes.NewBufferString(`{"model":"test-model","prompt":"hello","stream":true}`)
	resp, err := http.Post(api.URL+"/api/generate", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(resp.Body)
	var chunks []llamacpp.StreamChunk
	for {
		var chunk llamacpp.StreamChunk
		if err := decoder.Decode(&chunk); err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, chunk)
		if chunk.Done {
			break
		}
	}
	if len(chunks) == 0 || chunks[0].Response != "fake " {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestAPIChatProxiesToFakeLlamaServer(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())
	api := httptest.NewServer(handler)
	defer api.Close()

	body := bytes.NewBufferString(`{"model":"test-model","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	resp, err := http.Post(api.URL+"/api/chat", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload llamacpp.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Message.Content != "fake chat" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestAPIShowDeleteAndImport(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())
	api := httptest.NewServer(handler)
	defer api.Close()

	showResp, err := http.Post(api.URL+"/api/show", "application/json", bytes.NewBufferString(`{"name":"test-model"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer showResp.Body.Close()
	if showResp.StatusCode != http.StatusOK {
		t.Fatalf("show status = %d", showResp.StatusCode)
	}
	var manifest models.Manifest
	if err := json.NewDecoder(showResp.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "test-model" {
		t.Fatalf("manifest name = %q", manifest.Name)
	}
	source := manifest.Path

	deleteReq, err := http.NewRequest(http.MethodDelete, api.URL+"/api/delete", bytes.NewBufferString(`{"name":"test-model"}`))
	if err != nil {
		t.Fatal(err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", deleteResp.StatusCode)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source GGUF should remain after API delete: %v", err)
	}

	reimportResp, err := http.Post(api.URL+"/api/models/import", "application/json", bytes.NewBufferString(fmt.Sprintf(`{"name":"test-model","path":%q,"mode":"reference"}`, source)))
	if err != nil {
		t.Fatal(err)
	}
	defer reimportResp.Body.Close()
	if reimportResp.StatusCode != http.StatusOK {
		t.Fatalf("reimport status = %d", reimportResp.StatusCode)
	}
}

func TestAPISettingsRejectsUnsafeValues(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())
	api := httptest.NewServer(handler)
	defer api.Close()

	resp, err := http.Get(api.URL + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings GET status = %d", resp.StatusCode)
	}

	resp, err = http.Post(api.URL+"/api/settings", "application/json", bytes.NewBufferString(`{"server":{"host":"0.0.0.0"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("public bind status = %d, want 400", resp.StatusCode)
	}

	resp, err = http.Post(api.URL+"/api/settings", "application/json", bytes.NewBufferString(`{"privacy":{"telemetry":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("telemetry status = %d, want 400", resp.StatusCode)
	}

	resp, err = http.Post(api.URL+"/api/settings", "application/json", bytes.NewBufferString(`{"runtime":{"backend":"cpu","idle_timeout":"1m","ready_timeout":"45s","llama_openvino_bin":"C:\\tools\\llama-openvino.exe","llama_cpu_bin":"C:\\tools\\llama-cpu.exe","openvino_device":"GPU.0","health_path":"/health","extra_openvino_args":["--device","GPU"],"extra_cpu_args":["--threads","8"],"allow_unverified_flags":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("safe settings status = %d", resp.StatusCode)
	}
	var payload struct {
		Runtime struct {
			Backend              string   `json:"backend"`
			IdleTimeout          string   `json:"idle_timeout"`
			ReadyTimeout         string   `json:"ready_timeout"`
			LlamaOpenVINOBin     string   `json:"llama_openvino_bin"`
			LlamaCPUBin          string   `json:"llama_cpu_bin"`
			OpenVINODevice       string   `json:"openvino_device"`
			HealthPath           string   `json:"health_path"`
			ExtraOpenVINOArgs    []string `json:"extra_openvino_args"`
			ExtraCPUArgs         []string `json:"extra_cpu_args"`
			AllowUnverifiedFlags bool     `json:"allow_unverified_flags"`
		} `json:"runtime"`
		Persisted       bool `json:"persisted"`
		RestartRequired bool `json:"restart_required"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Runtime.Backend != "cpu" ||
		payload.Runtime.IdleTimeout != "1m0s" ||
		payload.Runtime.ReadyTimeout != "45s" ||
		payload.Runtime.LlamaOpenVINOBin != `C:\tools\llama-openvino.exe` ||
		payload.Runtime.LlamaCPUBin != `C:\tools\llama-cpu.exe` ||
		payload.Runtime.OpenVINODevice != "GPU.0" ||
		payload.Runtime.HealthPath != "/health" ||
		len(payload.Runtime.ExtraOpenVINOArgs) != 2 ||
		len(payload.Runtime.ExtraCPUArgs) != 2 ||
		!payload.Runtime.AllowUnverifiedFlags ||
		payload.Persisted ||
		!payload.RestartRequired {
		t.Fatalf("settings payload = %#v", payload)
	}
}

func TestAPILocalCORSAllowsDesktopAndLocalDevelopmentOrigins(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())

	tests := []struct {
		name   string
		origin string
	}{
		{name: "wails desktop", origin: "wails://wails.localhost"},
		{name: "localhost dev", origin: "http://localhost:5173"},
		{name: "loopback dev", origin: "http://127.0.0.1:5173"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/api/settings", nil)
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.origin {
				t.Fatalf("allow origin = %q, want %q", got, tt.origin)
			}
			if methods := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, http.MethodPost) {
				t.Fatalf("allow methods = %q, want POST included", methods)
			}
		})
	}
}

func TestAPILocalCORSRejectsExternalOrigin(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("external origin was allowed: %q", got)
	}
}

func TestAPIDeploymentStatusAndSelectCPU(t *testing.T) {
	t.Setenv("VINOLLAMA_FAKE_LLAMA_HELP", "1")
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())
	api := httptest.NewServer(handler)
	defer api.Close()

	resp, err := http.Get(api.URL + "/api/deployment")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deployment status = %d", resp.StatusCode)
	}
	var report struct {
		Reference  string `json:"reference"`
		BuildPlans []struct {
			Backend string `json:"backend"`
		} `json:"build_plans"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Reference == "" || len(report.BuildPlans) == 0 {
		t.Fatalf("deployment report = %#v", report)
	}

	body, err := json.Marshal(map[string]string{"kind": "cpu", "path": os.Args[0]})
	if err != nil {
		t.Fatal(err)
	}
	selectResp, err := http.Post(api.URL+"/api/deployment/select", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer selectResp.Body.Close()
	if selectResp.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(selectResp.Body)
		t.Fatalf("deployment select status = %d body=%s", selectResp.StatusCode, text)
	}
	var payload struct {
		Selected bool `json:"selected"`
		Settings struct {
			Runtime struct {
				LlamaCPUBin string `json:"llama_cpu_bin"`
			} `json:"runtime"`
		} `json:"settings"`
	}
	if err := json.NewDecoder(selectResp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Selected || payload.Settings.Runtime.LlamaCPUBin == "" {
		t.Fatalf("deployment select payload = %#v", payload)
	}
}

func TestAPILogsReturnsRuntimeLogTail(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())
	api := httptest.NewServer(handler)
	defer api.Close()

	if err := os.MkdirAll(manager.LogDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manager.LogDir(), "test.log"), []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(api.URL + "/api/logs?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logs status = %d", resp.StatusCode)
	}
	var payload struct {
		Logs []struct {
			File  string   `json:"file"`
			Lines []string `json:"lines"`
		} `json:"logs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Logs) != 1 || payload.Logs[0].File != "test.log" || len(payload.Logs[0].Lines) != 1 || payload.Logs[0].Lines[0] != "line two" {
		t.Fatalf("logs payload = %#v", payload)
	}
}

func TestAPIRuntimeRestartStartsModel(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())
	api := httptest.NewServer(handler)
	defer api.Close()

	resp, err := http.Post(api.URL+"/api/runtime/restart", "application/json", bytes.NewBufferString(`{"model":"test-model","backend":"cpu"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restart status = %d", resp.StatusCode)
	}
	var payload struct {
		Restarted       bool                   `json:"restarted"`
		StoppedExisting bool                   `json:"stopped_existing"`
		Process         llamacpp.ProcessHandle `json:"process"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Restarted || payload.Process.Model != "test-model" || payload.Process.State != llamacpp.ProcessReady {
		t.Fatalf("restart payload = %#v", payload)
	}
}

func TestAPIConversationsCRUDAndExport(t *testing.T) {
	handler, manager := newFakeAPIHandler(t)
	defer manager.ShutdownAll(context.Background())
	api := httptest.NewServer(handler)
	defer api.Close()

	createBody := `{"model":"test-model","messages":[{"role":"user","content":"Hello conversation"}]}`
	resp, err := http.Post(api.URL+"/api/conversations", "application/json", bytes.NewBufferString(createBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	var created struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Title != "Hello conversation" {
		t.Fatalf("created = %#v", created)
	}

	resp, err = http.Get(api.URL + "/api/conversations")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list struct {
		Conversations []struct {
			ID string `json:"id"`
		} `json:"conversations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Conversations) != 1 || list.Conversations[0].ID != created.ID {
		t.Fatalf("list = %#v", list)
	}

	req, err := http.NewRequest(http.MethodPut, api.URL+"/api/conversations/"+created.ID, bytes.NewBufferString(`{"title":"Renamed"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d", resp.StatusCode)
	}

	resp, err = http.Post(api.URL+"/api/conversations/"+created.ID+"/export", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var exported struct {
		Format  string `json:"format"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&exported); err != nil {
		t.Fatal(err)
	}
	if exported.Format != "markdown" || !strings.Contains(exported.Content, "# Renamed") {
		t.Fatalf("exported = %#v", exported)
	}

	req, err = http.NewRequest(http.MethodDelete, api.URL+"/api/conversations/"+created.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
}

func newFakeAPIHandler(t *testing.T) (http.Handler, *vinoruntime.Manager) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VINOLLAMA_FAKE_LLAMA_HELP", "1")
	t.Setenv("VINOLLAMA_FAKE_LLAMA_SERVER", "1")
	cfg := config.Defaults()
	cfg.Runtime.Backend = "cpu"
	cfg.Runtime.ReadyTimeout = 5 * time.Second
	cfg.Runtime.InternalPortStart = freeAPIPort(t)
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
	manager, err := vinoruntime.NewManager(vinoruntime.ManagerOptions{
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
	return NewHandler(cfg, manager, store), manager
}

func runAPIFakeLlamaServer() {
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
	mux.HandleFunc("/completion", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte("{\"content\":\"fake \",\"stop\":false}\n{\"content\":\"completion\",\"stop\":true}\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"fake completion"}`))
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"fake chat\"}}]}\n\ndata: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fake chat"}}]}`))
	})
	if err := http.Serve(listener, mux); err != nil && !strings.Contains(err.Error(), "closed") {
		fmt.Fprintf(os.Stderr, "server stopped: %v\n", err)
	}
}

func freeAPIPort(t *testing.T) int {
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
