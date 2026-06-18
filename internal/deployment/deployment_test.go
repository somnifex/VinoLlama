package deployment

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"vinollama/internal/config"
)

func TestSelectBinaryConfiguresOpenVINOBinary(t *testing.T) {
	binary := writeFakeServer(t, "llama-server-openvino", "llama.cpp server version 1.2.3 OpenVINO")
	cfg := config.Defaults()

	next, candidate, err := SelectBinary(context.Background(), cfg, "openvino", binary)
	if err != nil {
		t.Fatalf("SelectBinary returned error: %v", err)
	}
	if next.Runtime.LlamaOpenVINOBin != candidate.Path {
		t.Fatalf("openvino path = %q, want %q", next.Runtime.LlamaOpenVINOBin, candidate.Path)
	}
	if !candidate.Usable || !candidate.OpenVINOCapable {
		t.Fatalf("candidate = %#v", candidate)
	}
}

func TestSelectBinaryRejectsUnconfirmedOpenVINO(t *testing.T) {
	binary := writeFakeServer(t, "llama-server-cpu", "llama.cpp server version 1.2.3")
	cfg := config.Defaults()

	_, candidate, err := SelectBinary(context.Background(), cfg, "openvino", binary)
	if err == nil {
		t.Fatal("SelectBinary succeeded with an unconfirmed OpenVINO binary")
	}
	if !candidate.Usable || candidate.OpenVINOCapable {
		t.Fatalf("candidate = %#v", candidate)
	}
}

func TestInspectIncludesBuildPlansAndRecommendations(t *testing.T) {
	setTestHome(t)
	report := Inspect(context.Background(), config.Defaults())

	if report.Reference != OpenVINODocsURL {
		t.Fatalf("reference = %q", report.Reference)
	}
	if len(report.BuildPlans) == 0 {
		t.Fatal("expected build plans")
	}
	if len(report.Recommendations) == 0 {
		t.Fatal("expected recommendations")
	}
	if report.Managed.Root == "" {
		t.Fatal("expected managed install directory")
	}
	if report.Readiness == "" {
		t.Fatal("expected readiness")
	}
	if len(report.Actions) == 0 {
		t.Fatal("expected end-user deployment actions")
	}
}

func TestDeployBinaryCopiesToManagedDirectoryAndConfiguresCPU(t *testing.T) {
	home := setTestHome(t)
	binary := writeFakeServer(t, "llama-server-cpu", "llama.cpp server version 1.2.3")
	cfg := config.Defaults()

	next, candidate, err := DeployBinary(context.Background(), cfg, "cpu", binary)
	if err != nil {
		t.Fatalf("DeployBinary returned error: %v", err)
	}
	if !candidate.Usable || candidate.Source != "managed" {
		t.Fatalf("candidate = %#v", candidate)
	}
	if next.Runtime.LlamaCPUBin != candidate.Path {
		t.Fatalf("cpu path = %q, want %q", next.Runtime.LlamaCPUBin, candidate.Path)
	}
	wantDir := filepath.Join(home, ".vinollama", "bin", "cpu")
	if filepath.Dir(candidate.Path) != wantDir {
		t.Fatalf("managed dir = %q, want %q", filepath.Dir(candidate.Path), wantDir)
	}
	if _, err := os.Stat(candidate.Path); err != nil {
		t.Fatalf("managed binary was not copied: %v", err)
	}
}

func writeFakeServer(t *testing.T, name, help string) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, name+".bat")
		content := "@echo off\r\necho " + help + "\r\n"
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := filepath.Join(dir, name)
	content := "#!/bin/sh\necho '" + help + "'\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		return home
	}
	t.Setenv("HOME", home)
	return home
}
