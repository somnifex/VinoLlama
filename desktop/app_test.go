package main

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"vinollama/internal/config"
)

func TestAppStartsAndStopsManagedService(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.Port = freeDesktopPort(t)
	cfg.Models.Directory = t.TempDir()
	cfg.Desktop.StartServiceOnLaunch = false
	cfg.Desktop.StopServiceOnExit = true

	app := NewApp()
	app.cfg = cfg
	if err := app.startManagedService(context.Background()); err != nil {
		t.Fatalf("startManagedService returned error: %v", err)
	}
	status := waitForDesktopService(t, app, true)
	if !status.Running || status.BaseURL == "" {
		t.Fatalf("service status = %#v", status)
	}

	app.stopManagedService(context.Background(), true)
	status = waitForDesktopService(t, app, false)
	if status.Running {
		t.Fatalf("service should be stopped: %#v", status)
	}
}

func waitForDesktopService(t *testing.T, app *App, wantRunning bool) ServiceStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var status ServiceStatus
	for time.Now().Before(deadline) {
		status = app.ServiceStatus()
		if status.Running == wantRunning {
			return status
		}
		time.Sleep(25 * time.Millisecond)
	}
	return status
}

func freeDesktopPort(t *testing.T) int {
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
