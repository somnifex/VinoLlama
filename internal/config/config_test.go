package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultsArePrivacySafe(t *testing.T) {
	cfg := Defaults()

	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("default host = %q, want 127.0.0.1", cfg.Server.Host)
	}
	if cfg.Server.Port != 11435 {
		t.Fatalf("default port = %d, want 11435", cfg.Server.Port)
	}
	if cfg.Privacy.Telemetry {
		t.Fatal("telemetry must be disabled by default")
	}
	if cfg.Runtime.Backend != "auto" {
		t.Fatalf("default backend = %q, want auto", cfg.Runtime.Backend)
	}
	if cfg.Runtime.IdleTimeout != 10*time.Minute {
		t.Fatalf("default idle timeout = %s, want 10m", cfg.Runtime.IdleTimeout)
	}
	if cfg.Runtime.ReadyTimeout != 30*time.Second {
		t.Fatalf("default ready timeout = %s, want 30s", cfg.Runtime.ReadyTimeout)
	}
	if cfg.Runtime.InternalPortStart != 21435 {
		t.Fatalf("default internal port start = %d, want 21435", cfg.Runtime.InternalPortStart)
	}
	if cfg.Runtime.AllowUnverifiedFlags {
		t.Fatal("allow_unverified_flags must be disabled by default")
	}
	if cfg.Models.DefaultImportMode != "reference" {
		t.Fatalf("default import mode = %q, want reference", cfg.Models.DefaultImportMode)
	}
}

func TestLoadFileAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	content := []byte(`server:
  host: 127.0.0.1
  port: 12000
runtime:
  backend: cpu
  idle_timeout: 2m
  ready_timeout: 45s
  internal_port_start: 22000
  health_path: /healthz
  extra_cpu_args: ["--temp", "0.4"]
  allow_unverified_flags: true
models:
  directory: ` + filepath.Join(dir, "models") + `
privacy:
  telemetry: false
`)
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VINOLLAMA_BACKEND", "openvino")
	t.Setenv("VINOLLAMA_PORT", "13000")

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if !loaded.Found {
		t.Fatal("expected config file to be found")
	}
	if loaded.Config.Runtime.Backend != "openvino" {
		t.Fatalf("backend = %q, want openvino", loaded.Config.Runtime.Backend)
	}
	if loaded.Config.Server.Port != 13000 {
		t.Fatalf("port = %d, want 13000", loaded.Config.Server.Port)
	}
	if loaded.Config.Runtime.IdleTimeout != 2*time.Minute {
		t.Fatalf("idle timeout = %s, want 2m", loaded.Config.Runtime.IdleTimeout)
	}
	if loaded.Config.Runtime.ReadyTimeout != 45*time.Second {
		t.Fatalf("ready timeout = %s, want 45s", loaded.Config.Runtime.ReadyTimeout)
	}
	if loaded.Config.Runtime.InternalPortStart != 22000 {
		t.Fatalf("internal port start = %d, want 22000", loaded.Config.Runtime.InternalPortStart)
	}
	if loaded.Config.Runtime.HealthPath != "/healthz" {
		t.Fatalf("health path = %q, want /healthz", loaded.Config.Runtime.HealthPath)
	}
	if len(loaded.Config.Runtime.ExtraCPUArgs) != 2 || loaded.Config.Runtime.ExtraCPUArgs[0] != "--temp" {
		t.Fatalf("extra CPU args = %#v", loaded.Config.Runtime.ExtraCPUArgs)
	}
	if !loaded.Config.Runtime.AllowUnverifiedFlags {
		t.Fatal("allow_unverified_flags = false, want true")
	}
}

func TestLoadRejectsUnsafeBackendValue(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runtime:\n  backend: cloud\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected invalid backend error")
	}
}
