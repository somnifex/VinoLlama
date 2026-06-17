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
