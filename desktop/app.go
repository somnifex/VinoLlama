package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"vinollama/internal/config"
	"vinollama/internal/models"
	vinoruntime "vinollama/internal/runtime"
	"vinollama/internal/server"
)

type App struct {
	ctx     context.Context
	mu      sync.Mutex
	cfg     config.Config
	httpSrv *http.Server
	manager *vinoruntime.Manager
	lastErr string
}

type ServiceStatus struct {
	Running bool   `json:"running"`
	BaseURL string `json:"base_url"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	loaded, err := config.Load("")
	if err != nil {
		a.setServiceError(fmt.Sprintf("configuration could not be loaded: %v", err))
		a.cfg = safeDesktopDefaults()
		return
	}
	cfg := loaded.Config
	cfg = enforceDesktopLocalBind(cfg)
	a.cfg = cfg
	if cfg.Desktop.StartServiceOnLaunch {
		if err := a.startManagedService(context.Background()); err != nil {
			a.setServiceError(err.Error())
		}
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.stopManagedService(ctx, a.cfg.Desktop.StopServiceOnExit)
}

func (a *App) ServiceStatus() ServiceStatus {
	baseURL := a.baseURL()
	if !a.probeService(baseURL).Running && a.cfg.Desktop.StartServiceOnLaunch {
		_ = a.startManagedService(context.Background())
	}
	reqCtx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/api/version", nil)
	if err != nil {
		return ServiceStatus{Running: false, BaseURL: baseURL, Error: err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if serviceErr := a.serviceError(); serviceErr != "" {
			return ServiceStatus{Running: false, BaseURL: baseURL, Error: serviceErr}
		}
		return ServiceStatus{Running: false, BaseURL: baseURL, Error: "local service is not running"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ServiceStatus{Running: false, BaseURL: baseURL, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	var payload struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ServiceStatus{Running: false, BaseURL: baseURL, Error: err.Error()}
	}
	return ServiceStatus{Running: true, BaseURL: baseURL, Name: payload.Name, Version: payload.Version}
}

func (a *App) StartService() ServiceStatus {
	if err := a.startManagedService(context.Background()); err != nil {
		a.setServiceError(err.Error())
	}
	return a.ServiceStatus()
}

func (a *App) StopService(stopRuntime bool) ServiceStatus {
	a.stopManagedService(context.Background(), stopRuntime)
	return a.ServiceStatus()
}

func (a *App) startManagedService(ctx context.Context) error {
	a.mu.Lock()
	if a.httpSrv != nil {
		a.mu.Unlock()
		return nil
	}
	cfg := a.cfg
	if cfg.Server.Host == "" || cfg.Server.Port == 0 {
		cfg = safeDesktopDefaults()
		a.cfg = cfg
	}
	baseURL := baseURLForConfig(cfg)
	a.mu.Unlock()

	if status := a.probeService(baseURL); status.Running {
		a.setServiceError("")
		return nil
	}

	store, err := models.NewStore(mustModelsDirectory(cfg))
	if err != nil {
		return fmt.Errorf("model store could not be opened: %w", err)
	}
	manager, err := vinoruntime.NewManager(vinoruntime.ManagerOptions{Config: cfg, Store: store})
	if err != nil {
		return fmt.Errorf("runtime manager could not be initialized: %w", err)
	}
	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		_ = manager.ShutdownAll(ctx)
		return fmt.Errorf("local API could not bind %s: %w", addr, err)
	}
	httpSrv := &http.Server{
		Addr: addr,
		Handler: server.NewHandlerWithOptions(cfg, manager, store, server.HandlerOptions{
			OnConfigUpdate: a.updateConfig,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	a.mu.Lock()
	if a.httpSrv != nil {
		a.mu.Unlock()
		_ = listener.Close()
		_ = manager.ShutdownAll(ctx)
		return nil
	}
	a.httpSrv = httpSrv
	a.manager = manager
	a.lastErr = ""
	a.mu.Unlock()

	go func() {
		if err := httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
			a.setServiceError(fmt.Sprintf("local API stopped unexpectedly: %v", err))
		}
	}()
	return nil
}

func (a *App) stopManagedService(ctx context.Context, stopRuntime bool) {
	a.mu.Lock()
	httpSrv := a.httpSrv
	manager := a.manager
	a.httpSrv = nil
	a.manager = nil
	a.mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if httpSrv != nil {
		_ = httpSrv.Shutdown(shutdownCtx)
	}
	if stopRuntime && manager != nil {
		_ = manager.ShutdownAll(shutdownCtx)
	}
}

func (a *App) probeService(baseURL string) ServiceStatus {
	reqCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/api/version", nil)
	if err != nil {
		return ServiceStatus{Running: false, BaseURL: baseURL, Error: err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ServiceStatus{Running: false, BaseURL: baseURL, Error: err.Error()}
	}
	defer resp.Body.Close()
	return ServiceStatus{Running: resp.StatusCode >= 200 && resp.StatusCode < 300, BaseURL: baseURL}
}

func (a *App) baseURL() string {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	if cfg.Server.Host == "" || cfg.Server.Port == 0 {
		cfg = safeDesktopDefaults()
	}
	return baseURLForConfig(cfg)
}

func (a *App) setServiceError(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastErr = message
}

func (a *App) updateConfig(cfg config.Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = enforceDesktopLocalBind(cfg)
}

func (a *App) serviceError() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastErr
}

func safeDesktopDefaults() config.Config {
	return enforceDesktopLocalBind(config.Defaults())
}

func enforceDesktopLocalBind(cfg config.Config) config.Config {
	if cfg.Server.Host == "" || cfg.Server.Host == "0.0.0.0" || cfg.Server.Host == "::" {
		cfg.Server.Host = config.DefaultHost
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = config.DefaultPort
	}
	return cfg
}

func baseURLForConfig(cfg config.Config) string {
	return fmt.Sprintf("http://%s", net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port)))
}

func mustModelsDirectory(cfg config.Config) string {
	dir, err := config.ModelsDirectory(cfg)
	if err != nil {
		root, rootErr := config.DefaultRootDir()
		if rootErr != nil {
			return "models"
		}
		return filepath.Join(root, "models")
	}
	return dir
}
