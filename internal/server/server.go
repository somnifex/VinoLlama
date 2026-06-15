package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"vinollama/internal/config"
	"vinollama/internal/conversations"
	"vinollama/internal/diagnostic"
	"vinollama/internal/llamacpp"
	"vinollama/internal/models"
	vinoruntime "vinollama/internal/runtime"
)

type Server struct {
	cfg     config.Config
	manager *vinoruntime.Manager
	store   models.Store
	convs   conversations.Store
}

func NewHandler(cfg config.Config, manager *vinoruntime.Manager, store models.Store) http.Handler {
	conversationDir, _ := config.ConversationsDirectory(cfg)
	convs, _ := conversations.NewStore(conversationDir)
	s := &Server{cfg: cfg, manager: manager, store: store, convs: convs}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/tags", s.handleTags)
	mux.HandleFunc("/api/show", s.handleShow)
	mux.HandleFunc("/api/delete", s.handleDelete)
	mux.HandleFunc("/api/generate", s.handleGenerate)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/runtime", s.handleRuntime)
	mux.HandleFunc("/api/runtime/stop", s.handleRuntimeStop)
	mux.HandleFunc("/api/runtime/restart", s.handleRuntimeRestart)
	mux.HandleFunc("/api/doctor", s.handleDoctor)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/models/import", s.handleModelsImport)
	mux.HandleFunc("/api/conversations", s.handleConversations)
	mux.HandleFunc("/api/conversations/", s.handleConversationByID)
	return mux
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only GET is supported.", "Use GET /api/version.", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": "0.1.0", "name": "VinoLlama"})
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only GET is supported.", "Use GET /api/tags.", "")
		return
	}
	manifests, err := s.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Models could not be listed.", err.Error(), "Check that the model manifest directory is readable.", "")
		return
	}
	type modelInfo struct {
		Name       string    `json:"name"`
		Size       int64     `json:"size"`
		ModifiedAt time.Time `json:"modified_at"`
	}
	models := make([]modelInfo, 0, len(manifests))
	for _, manifest := range manifests {
		models = append(models, modelInfo{Name: manifest.Name, Size: manifest.Size, ModifiedAt: manifest.ModifiedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only POST is supported.", "Use POST /api/show.", "")
		return
	}
	name, err := modelNameFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Show request is invalid.", err.Error(), "Send JSON like {\"name\":\"model\"}.", "")
		return
	}
	manifest, err := s.store.ReadManifest(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "Model could not be shown.", err.Error(), "Import the model first with `vinollama import`.", fmt.Sprintf("model=%s", name))
		return
	}
	writeJSON(w, http.StatusOK, manifest)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only DELETE is supported.", "Use DELETE /api/delete.", "")
		return
	}
	var req struct {
		Name       string `json:"name"`
		Model      string `json:"model"`
		DeleteFile bool   `json:"delete_file"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(req.Model)
	}
	if name == "" {
		name = strings.TrimSpace(r.URL.Query().Get("name"))
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "Delete request is invalid.", "model name is empty", "Send JSON like {\"name\":\"model\"}.", "")
		return
	}
	result, err := s.store.Delete(name, req.DeleteFile)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Model could not be deleted.", err.Error(), "Check the model name and only use delete_file when you really want to remove the GGUF file.", fmt.Sprintf("model=%s", name))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":       true,
		"manifest_path": result.ManifestPath,
		"model_path":    result.ModelPath,
		"file_deleted":  result.FileDeleted,
	})
}

func (s *Server) handleRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only GET is supported.", "Use GET /api/runtime.", "")
		return
	}
	if s.manager == nil {
		writeJSON(w, http.StatusOK, map[string]any{"processes": []llamacpp.ProcessHandle{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"processes": s.manager.ListProcesses()})
}

func (s *Server) handleRuntimeStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only POST is supported.", "Use POST /api/runtime/stop.", "")
		return
	}
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Runtime stop request could not be decoded.", err.Error(), "Send JSON like {\"model\":\"name\"}.", "")
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "Runtime stop request is invalid.", "model is empty", "Pass the model name to stop.", "")
		return
	}
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "Runtime manager is unavailable.", "server was started without a runtime manager", "Restart VinoLlama with runtime initialization enabled.", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	stopped, err := s.manager.StopModel(ctx, req.Model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Model process could not be stopped.", err.Error(), "Check runtime logs and process state.", fmt.Sprintf("model=%s", req.Model))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopped": stopped})
}

func (s *Server) handleRuntimeRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only POST is supported.", "Use POST /api/runtime/restart.", "")
		return
	}
	var req struct {
		Model   string `json:"model"`
		Backend string `json:"backend"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Runtime restart request could not be decoded.", err.Error(), "Send JSON like {\"model\":\"name\"}.", "")
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "Runtime restart request is invalid.", "model is empty", "Pass the model name to restart.", "")
		return
	}
	if strings.TrimSpace(req.Backend) != "" && !config.ValidBackend(strings.TrimSpace(req.Backend)) {
		writeError(w, http.StatusBadRequest, "Runtime restart request is invalid.", fmt.Sprintf("unsupported backend %q", req.Backend), "Use one of: auto, openvino, cpu.", "")
		return
	}
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "Runtime manager is unavailable.", "server was started without a runtime manager", "Restart VinoLlama with runtime initialization enabled.", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	handle, stopped, err := s.manager.RestartModel(ctx, req.Model, vinoruntime.StartOptions{Backend: strings.TrimSpace(req.Backend)})
	if err != nil {
		writeRuntimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restarted": true, "stopped_existing": stopped, "process": handle.Snapshot()})
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only POST is supported.", "Use POST /api/generate.", "")
		return
	}
	var req llamacpp.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Generate request could not be decoded.", err.Error(), "Send a valid JSON generate request.", "")
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "Generate request is invalid.", "model is empty", "Import a model and pass its name in the request.", "")
		return
	}
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "Runtime manager is unavailable.", "server was started without a runtime manager", "Restart VinoLlama with runtime initialization enabled.", "")
		return
	}
	if req.Stream {
		ch, err := s.manager.ProxyGenerateStream(r.Context(), req)
		if err != nil {
			writeRuntimeError(w, err)
			return
		}
		writeStream(w, ch)
		return
	}
	resp, err := s.manager.ProxyGenerate(r.Context(), req)
	if err != nil {
		writeRuntimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only POST is supported.", "Use POST /api/chat.", "")
		return
	}
	var req llamacpp.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Chat request could not be decoded.", err.Error(), "Send a valid JSON chat request.", "")
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "Chat request is invalid.", "model is empty", "Import a model and pass its name in the request.", "")
		return
	}
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "Runtime manager is unavailable.", "server was started without a runtime manager", "Restart VinoLlama with runtime initialization enabled.", "")
		return
	}
	if req.Stream {
		ch, err := s.manager.ProxyChatStream(r.Context(), req)
		if err != nil {
			writeRuntimeError(w, err)
			return
		}
		writeStream(w, ch)
		return
	}
	resp, err := s.manager.ProxyChat(r.Context(), req)
	if err != nil {
		writeRuntimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only GET is supported.", "Use GET /api/doctor.", "")
		return
	}
	report := diagnostic.Run(r.Context(), s.cfg, "", false)
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, settingsResponse(s.cfg, false, false))
	case http.MethodPost:
		var patch settingsPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeError(w, http.StatusBadRequest, "Settings request could not be decoded.", err.Error(), "Send a valid JSON settings patch.", "")
			return
		}
		next, err := applySettingsPatch(s.cfg, patch)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Settings request is invalid.", err.Error(), "Use safe local-first values and keep telemetry disabled.", "")
			return
		}
		s.cfg = next
		writeJSON(w, http.StatusOK, settingsResponse(s.cfg, false, true))
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only GET and POST are supported.", "Use GET or POST /api/settings.", "")
	}
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only GET is supported.", "Use GET /api/logs.", "")
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "Logs request is invalid.", "limit must be a positive integer", "Use /api/logs?limit=200.", "")
			return
		}
		if parsed < 1000 {
			limit = parsed
		} else {
			limit = 1000
		}
	}
	logDir := ""
	if s.manager != nil {
		logDir = s.manager.LogDir()
	}
	logs, err := readRecentLogs(logDir, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Logs could not be read.", err.Error(), "Check runtime log directory permissions.", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (s *Server) handleModelsImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only POST is supported.", "Use POST /api/models/import.", "")
		return
	}
	var req struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Model import request could not be decoded.", err.Error(), "Send JSON like {\"name\":\"model\",\"path\":\"file.gguf\",\"mode\":\"reference\"}.", "")
		return
	}
	if req.Mode == "" {
		req.Mode = s.cfg.Models.DefaultImportMode
	}
	manifest, err := s.store.Import(models.ImportRequest{Name: req.Name, Path: req.Path, Mode: req.Mode})
	if err != nil {
		writeError(w, http.StatusBadRequest, "Model could not be imported.", err.Error(), "Confirm the path points to a local GGUF file and the model directory is writable.", "")
		return
	}
	writeJSON(w, http.StatusOK, manifest)
}

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.convs.List()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Conversations could not be listed.", err.Error(), "Check conversation directory permissions.", "")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"conversations": list})
	case http.MethodPost:
		var req struct {
			Title    string                  `json:"title"`
			Model    string                  `json:"model"`
			Messages []conversations.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Conversation request could not be decoded.", err.Error(), "Send a valid JSON conversation.", "")
			return
		}
		conv, err := s.convs.Create(conversations.CreateRequest{Title: req.Title, Model: req.Model, Messages: req.Messages})
		if err != nil {
			writeError(w, http.StatusBadRequest, "Conversation could not be created.", err.Error(), "Check message roles and conversation directory permissions.", "")
			return
		}
		writeJSON(w, http.StatusOK, conv)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only GET and POST are supported.", "Use GET or POST /api/conversations.", "")
	}
}

func (s *Server) handleConversationByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseConversationPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "Conversation route was not found.", r.URL.Path, "Use /api/conversations/{id} or /api/conversations/{id}/export.", "")
		return
	}
	if action == "export" {
		s.handleConversationExport(w, r, id)
		return
	}
	switch r.Method {
	case http.MethodGet:
		conv, err := s.convs.Read(id)
		if err != nil {
			writeError(w, http.StatusNotFound, "Conversation could not be read.", err.Error(), "Check the conversation id.", fmt.Sprintf("id=%s", id))
			return
		}
		writeJSON(w, http.StatusOK, conv)
	case http.MethodPut:
		var req struct {
			Title    *string                 `json:"title"`
			Model    *string                 `json:"model"`
			Messages []conversations.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Conversation update could not be decoded.", err.Error(), "Send a valid JSON conversation patch.", "")
			return
		}
		conv, err := s.convs.Update(id, conversations.UpdateRequest{Title: req.Title, Model: req.Model, Messages: req.Messages})
		if err != nil {
			writeError(w, http.StatusBadRequest, "Conversation could not be updated.", err.Error(), "Check the conversation id and message roles.", fmt.Sprintf("id=%s", id))
			return
		}
		writeJSON(w, http.StatusOK, conv)
	case http.MethodDelete:
		if err := s.convs.Delete(id); err != nil {
			writeError(w, http.StatusBadRequest, "Conversation could not be deleted.", err.Error(), "Check the conversation id.", fmt.Sprintf("id=%s", id))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "GET, PUT, and DELETE are supported.", "Use /api/conversations/{id}.", "")
	}
}

func (s *Server) handleConversationExport(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "Only POST is supported.", "Use POST /api/conversations/{id}/export.", "")
		return
	}
	content, err := s.convs.ExportMarkdown(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Conversation could not be exported.", err.Error(), "Check the conversation id.", fmt.Sprintf("id=%s", id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "format": "markdown", "content": content})
}

func parseConversationPath(path string) (id, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/conversations/")
	if rest == path || rest == "" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", "", false
	}
	if len(parts) > 2 {
		return "", "", false
	}
	if len(parts) == 2 {
		if parts[1] != "export" {
			return "", "", false
		}
		action = "export"
	}
	return parts[0], action, true
}

func modelNameFromRequest(r *http.Request) (string, error) {
	var req struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return "", err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(req.Model)
	}
	if name == "" {
		return "", fmt.Errorf("model name is empty")
	}
	return name, nil
}

type settingsPayload struct {
	Server struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"server"`
	Runtime struct {
		Backend              string   `json:"backend"`
		IdleTimeout          string   `json:"idle_timeout"`
		ReadyTimeout         string   `json:"ready_timeout"`
		LlamaOpenVINOBin     string   `json:"llama_openvino_bin"`
		LlamaCPUBin          string   `json:"llama_cpu_bin"`
		InternalPortStart    int      `json:"internal_port_start"`
		HealthPath           string   `json:"health_path"`
		ExtraOpenVINOArgs    []string `json:"extra_openvino_args"`
		ExtraCPUArgs         []string `json:"extra_cpu_args"`
		AllowUnverifiedFlags bool     `json:"allow_unverified_flags"`
	} `json:"runtime"`
	Generation struct {
		CtxSize     int     `json:"ctx_size"`
		Temperature float64 `json:"temperature"`
		TopP        float64 `json:"top_p"`
		Threads     int     `json:"threads"`
	} `json:"generation"`
	Models struct {
		Directory         string `json:"directory"`
		DefaultImportMode string `json:"default_import_mode"`
	} `json:"models"`
	Desktop struct {
		StartServiceOnLaunch bool   `json:"start_service_on_launch"`
		StopServiceOnExit    bool   `json:"stop_service_on_exit"`
		Theme                string `json:"theme"`
		CompactMode          bool   `json:"compact_mode"`
	} `json:"desktop"`
	Privacy struct {
		Telemetry bool `json:"telemetry"`
	} `json:"privacy"`
	Logging struct {
		Level string `json:"level"`
		File  string `json:"file"`
	} `json:"logging"`
	Persisted       bool `json:"persisted"`
	RestartRequired bool `json:"restart_required"`
}

type settingsPatch struct {
	Server *struct {
		Host *string `json:"host"`
		Port *int    `json:"port"`
	} `json:"server"`
	Runtime *struct {
		Backend              *string  `json:"backend"`
		IdleTimeout          *string  `json:"idle_timeout"`
		ReadyTimeout         *string  `json:"ready_timeout"`
		LlamaOpenVINOBin     *string  `json:"llama_openvino_bin"`
		LlamaCPUBin          *string  `json:"llama_cpu_bin"`
		InternalPortStart    *int     `json:"internal_port_start"`
		HealthPath           *string  `json:"health_path"`
		ExtraOpenVINOArgs    []string `json:"extra_openvino_args"`
		ExtraCPUArgs         []string `json:"extra_cpu_args"`
		AllowUnverifiedFlags *bool    `json:"allow_unverified_flags"`
	} `json:"runtime"`
	Generation *struct {
		CtxSize     *int     `json:"ctx_size"`
		Temperature *float64 `json:"temperature"`
		TopP        *float64 `json:"top_p"`
		Threads     *int     `json:"threads"`
	} `json:"generation"`
	Models *struct {
		Directory         *string `json:"directory"`
		DefaultImportMode *string `json:"default_import_mode"`
	} `json:"models"`
	Desktop *struct {
		StartServiceOnLaunch *bool   `json:"start_service_on_launch"`
		StopServiceOnExit    *bool   `json:"stop_service_on_exit"`
		Theme                *string `json:"theme"`
		CompactMode          *bool   `json:"compact_mode"`
	} `json:"desktop"`
	Privacy *struct {
		Telemetry *bool `json:"telemetry"`
	} `json:"privacy"`
	Logging *struct {
		Level *string `json:"level"`
		File  *string `json:"file"`
	} `json:"logging"`
}

func settingsResponse(cfg config.Config, persisted, restartRequired bool) settingsPayload {
	var payload settingsPayload
	payload.Server.Host = cfg.Server.Host
	payload.Server.Port = cfg.Server.Port
	payload.Runtime.Backend = cfg.Runtime.Backend
	payload.Runtime.IdleTimeout = cfg.Runtime.IdleTimeout.String()
	payload.Runtime.ReadyTimeout = cfg.Runtime.ReadyTimeout.String()
	payload.Runtime.LlamaOpenVINOBin = cfg.Runtime.LlamaOpenVINOBin
	payload.Runtime.LlamaCPUBin = cfg.Runtime.LlamaCPUBin
	payload.Runtime.InternalPortStart = cfg.Runtime.InternalPortStart
	payload.Runtime.HealthPath = cfg.Runtime.HealthPath
	payload.Runtime.ExtraOpenVINOArgs = append([]string{}, cfg.Runtime.ExtraOpenVINOArgs...)
	payload.Runtime.ExtraCPUArgs = append([]string{}, cfg.Runtime.ExtraCPUArgs...)
	payload.Runtime.AllowUnverifiedFlags = cfg.Runtime.AllowUnverifiedFlags
	payload.Generation.CtxSize = cfg.Generation.CtxSize
	payload.Generation.Temperature = cfg.Generation.Temperature
	payload.Generation.TopP = cfg.Generation.TopP
	payload.Generation.Threads = cfg.Generation.Threads
	payload.Models.Directory = cfg.Models.Directory
	payload.Models.DefaultImportMode = cfg.Models.DefaultImportMode
	payload.Desktop.StartServiceOnLaunch = cfg.Desktop.StartServiceOnLaunch
	payload.Desktop.StopServiceOnExit = cfg.Desktop.StopServiceOnExit
	payload.Desktop.Theme = cfg.Desktop.Theme
	payload.Desktop.CompactMode = cfg.Desktop.CompactMode
	payload.Privacy.Telemetry = cfg.Privacy.Telemetry
	payload.Logging.Level = cfg.Logging.Level
	payload.Logging.File = cfg.Logging.File
	payload.Persisted = persisted
	payload.RestartRequired = restartRequired
	return payload
}

func applySettingsPatch(cfg config.Config, patch settingsPatch) (config.Config, error) {
	next := cfg
	if patch.Server != nil {
		if patch.Server.Host != nil {
			host := strings.TrimSpace(*patch.Server.Host)
			if host == "" {
				return cfg, fmt.Errorf("server.host must not be empty")
			}
			if host == "0.0.0.0" || host == "::" {
				return cfg, fmt.Errorf("server.host must not expose VinoLlama beyond localhost")
			}
			next.Server.Host = host
		}
		if patch.Server.Port != nil {
			if *patch.Server.Port <= 0 || *patch.Server.Port > 65535 {
				return cfg, fmt.Errorf("server.port must be between 1 and 65535")
			}
			next.Server.Port = *patch.Server.Port
		}
	}
	if patch.Runtime != nil {
		if patch.Runtime.Backend != nil {
			backend := strings.TrimSpace(*patch.Runtime.Backend)
			if !config.ValidBackend(backend) {
				return cfg, fmt.Errorf("runtime.backend must be one of auto, openvino, cpu")
			}
			next.Runtime.Backend = backend
		}
		if patch.Runtime.IdleTimeout != nil {
			duration, err := time.ParseDuration(*patch.Runtime.IdleTimeout)
			if err != nil {
				return cfg, fmt.Errorf("runtime.idle_timeout must be a duration like 10m: %w", err)
			}
			next.Runtime.IdleTimeout = duration
		}
		if patch.Runtime.ReadyTimeout != nil {
			duration, err := time.ParseDuration(*patch.Runtime.ReadyTimeout)
			if err != nil {
				return cfg, fmt.Errorf("runtime.ready_timeout must be a duration like 30s: %w", err)
			}
			next.Runtime.ReadyTimeout = duration
		}
		if patch.Runtime.LlamaOpenVINOBin != nil {
			next.Runtime.LlamaOpenVINOBin = strings.TrimSpace(*patch.Runtime.LlamaOpenVINOBin)
		}
		if patch.Runtime.LlamaCPUBin != nil {
			next.Runtime.LlamaCPUBin = strings.TrimSpace(*patch.Runtime.LlamaCPUBin)
		}
		if patch.Runtime.InternalPortStart != nil {
			if *patch.Runtime.InternalPortStart <= 0 || *patch.Runtime.InternalPortStart > 65535 {
				return cfg, fmt.Errorf("runtime.internal_port_start must be between 1 and 65535")
			}
			next.Runtime.InternalPortStart = *patch.Runtime.InternalPortStart
		}
		if patch.Runtime.HealthPath != nil {
			next.Runtime.HealthPath = strings.TrimSpace(*patch.Runtime.HealthPath)
		}
		if patch.Runtime.ExtraOpenVINOArgs != nil {
			next.Runtime.ExtraOpenVINOArgs = append([]string{}, patch.Runtime.ExtraOpenVINOArgs...)
		}
		if patch.Runtime.ExtraCPUArgs != nil {
			next.Runtime.ExtraCPUArgs = append([]string{}, patch.Runtime.ExtraCPUArgs...)
		}
		if patch.Runtime.AllowUnverifiedFlags != nil {
			next.Runtime.AllowUnverifiedFlags = *patch.Runtime.AllowUnverifiedFlags
		}
	}
	if patch.Generation != nil {
		if patch.Generation.CtxSize != nil {
			if *patch.Generation.CtxSize <= 0 {
				return cfg, fmt.Errorf("generation.ctx_size must be positive")
			}
			next.Generation.CtxSize = *patch.Generation.CtxSize
		}
		if patch.Generation.Temperature != nil {
			next.Generation.Temperature = *patch.Generation.Temperature
		}
		if patch.Generation.TopP != nil {
			next.Generation.TopP = *patch.Generation.TopP
		}
		if patch.Generation.Threads != nil {
			if *patch.Generation.Threads < 0 {
				return cfg, fmt.Errorf("generation.threads must not be negative")
			}
			next.Generation.Threads = *patch.Generation.Threads
		}
	}
	if patch.Models != nil {
		if patch.Models.Directory != nil {
			next.Models.Directory = strings.TrimSpace(*patch.Models.Directory)
		}
		if patch.Models.DefaultImportMode != nil {
			mode := strings.TrimSpace(*patch.Models.DefaultImportMode)
			if !models.ValidImportMode(mode) {
				return cfg, fmt.Errorf("models.default_import_mode must be one of reference, copy, link")
			}
			next.Models.DefaultImportMode = mode
		}
	}
	if patch.Desktop != nil {
		if patch.Desktop.StartServiceOnLaunch != nil {
			next.Desktop.StartServiceOnLaunch = *patch.Desktop.StartServiceOnLaunch
		}
		if patch.Desktop.StopServiceOnExit != nil {
			next.Desktop.StopServiceOnExit = *patch.Desktop.StopServiceOnExit
		}
		if patch.Desktop.Theme != nil {
			next.Desktop.Theme = strings.TrimSpace(*patch.Desktop.Theme)
		}
		if patch.Desktop.CompactMode != nil {
			next.Desktop.CompactMode = *patch.Desktop.CompactMode
		}
	}
	if patch.Privacy != nil && patch.Privacy.Telemetry != nil {
		if *patch.Privacy.Telemetry {
			return cfg, fmt.Errorf("privacy.telemetry is disabled in the initial implementation")
		}
		next.Privacy.Telemetry = false
	}
	if patch.Logging != nil {
		if patch.Logging.Level != nil {
			level := strings.TrimSpace(*patch.Logging.Level)
			switch level {
			case "debug", "info", "warn", "error":
				next.Logging.Level = level
			default:
				return cfg, fmt.Errorf("logging.level must be debug, info, warn, or error")
			}
		}
		if patch.Logging.File != nil {
			next.Logging.File = strings.TrimSpace(*patch.Logging.File)
		}
	}
	return next, nil
}

type logEntry struct {
	File       string    `json:"file"`
	ModifiedAt time.Time `json:"modified_at"`
	Lines      []string  `json:"lines"`
}

func readRecentLogs(logDir string, limit int) ([]logEntry, error) {
	if strings.TrimSpace(logDir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type fileInfo struct {
		path    string
		name    string
		modTime time.Time
	}
	var files []fileInfo
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".log" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{path: filepath.Join(logDir, entry.Name()), name: entry.Name(), modTime: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	var out []logEntry
	remaining := limit
	for _, file := range files {
		if remaining <= 0 {
			break
		}
		lines, err := tailLines(file.path, remaining)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			continue
		}
		out = append(out, logEntry{File: file.name, ModifiedAt: file.modTime, Lines: lines})
		remaining -= len(lines)
	}
	return out, nil
}

func tailLines(path string, limit int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	const maxBytes int64 = 64 * 1024
	start := info.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return nil, nil
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeStream(w http.ResponseWriter, ch <-chan llamacpp.StreamChunk) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	encoder := json.NewEncoder(w)
	for chunk := range ch {
		_ = encoder.Encode(chunk)
		if flusher != nil {
			flusher.Flush()
		}
		if chunk.Error != "" || chunk.Done {
			return
		}
	}
}

func writeRuntimeError(w http.ResponseWriter, err error) {
	writeError(w, http.StatusBadGateway, "Runtime request failed.", err.Error(), "Run `vinollama doctor` and inspect llama.cpp runtime logs.", "")
}

func writeError(w http.ResponseWriter, status int, what, reason, fix, details string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"what":    what,
			"reason":  reason,
			"fix":     fix,
			"details": details,
		},
	})
}
