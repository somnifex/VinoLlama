package conversations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateListUpdateExportDelete(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.Create(CreateRequest{
		Model: "test-model",
		Messages: []Message{{
			Role:    "user",
			Content: "Hello from a local conversation",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if conv.Title != "Hello from a local conversation" {
		t.Fatalf("title = %q", conv.Title)
	}

	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != conv.ID || list[0].MessageCount != 1 {
		t.Fatalf("list = %#v", list)
	}

	title := "Renamed"
	updated, err := store.Update(conv.ID, UpdateRequest{Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Renamed" {
		t.Fatalf("updated title = %q", updated.Title)
	}

	markdown, err := store.ExportMarkdown(conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "# Renamed") || !strings.Contains(markdown, "Hello from a local conversation") {
		t.Fatalf("markdown = %q", markdown)
	}

	if err := store.Delete(conv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.path(conv.ID)); !os.IsNotExist(err) {
		t.Fatalf("conversation file should be removed, stat err = %v", err)
	}
}

func TestCleanIDRejectsPathTraversal(t *testing.T) {
	if _, err := CleanID("..\\outside"); err == nil {
		t.Fatal("expected path traversal rejection")
	}
}
