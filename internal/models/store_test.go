package models

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferMetadataFromFilename(t *testing.T) {
	got := InferMetadata("qwen2.5-7b-instruct-q4_k_m.gguf")
	if got.Parameters != "7B" {
		t.Fatalf("Parameters = %q, want 7B", got.Parameters)
	}
	if got.Quantization != "Q4_K_M" {
		t.Fatalf("Quantization = %q, want Q4_K_M", got.Quantization)
	}
}

func TestImportReferenceWritesManifest(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "qwen2.5-7b-instruct-q4_k_m.gguf")
	if err := os.WriteFile(source, []byte("GGUF fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := store.Import(ImportRequest{Name: "test-model", Path: source, Mode: SourceReference})
	if err != nil {
		t.Fatal(err)
	}

	if manifest.Source != SourceReference {
		t.Fatalf("Source = %q, want reference", manifest.Source)
	}
	if manifest.Path != source {
		t.Fatalf("Path = %q, want %q", manifest.Path, source)
	}
	if manifest.Parameters != "7B" || manifest.Quantization != "Q4_K_M" {
		t.Fatalf("metadata = %s/%s, want 7B/Q4_K_M", manifest.Parameters, manifest.Quantization)
	}
	if _, err := os.Stat(store.ManifestPath("test-model")); err != nil {
		t.Fatalf("manifest was not written: %v", err)
	}
}

func TestListAndDeleteManifestOnly(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "tiny-1b-q8_0.gguf")
	if err := os.WriteFile(source, []byte("GGUF fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(ImportRequest{Name: "tiny", Path: source, Mode: SourceReference}); err != nil {
		t.Fatal(err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "tiny" {
		t.Fatalf("List() = %#v, want one tiny manifest", list)
	}

	if _, err := store.Delete("tiny", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source file should remain after manifest-only delete: %v", err)
	}
	if _, err := os.Stat(store.ManifestPath("tiny")); !os.IsNotExist(err) {
		t.Fatalf("manifest should be removed, stat err = %v", err)
	}
}

func TestImportRejectsExistingManifest(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "tiny-1b-q8_0.gguf")
	if err := os.WriteFile(source, []byte("GGUF fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(ImportRequest{Name: "tiny", Path: source, Mode: SourceReference}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(ImportRequest{Name: "tiny", Path: source, Mode: SourceReference}); err == nil {
		t.Fatal("expected duplicate import to fail")
	}
}

func TestCleanNameRejectsPathTraversal(t *testing.T) {
	if _, err := CleanName("..\\outside"); err == nil {
		t.Fatal("expected path separator rejection")
	}
}
