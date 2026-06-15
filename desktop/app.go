//go:build wails

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type App struct {
	ctx context.Context
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
}

func (a *App) ServiceStatus() ServiceStatus {
	const baseURL = "http://127.0.0.1:11435"
	reqCtx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/api/version", nil)
	if err != nil {
		return ServiceStatus{Running: false, BaseURL: baseURL, Error: err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
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
