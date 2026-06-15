package llamacpp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPClientGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completion" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"hello from llama"}`))
	}))
	defer server.Close()

	resp, err := NewHTTPClient().Generate(context.Background(), server.URL, GenerateRequest{Model: "m", Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Response != "hello from llama" || !resp.Done {
		t.Fatalf("Generate() = %#v", resp)
	}
}

func TestHTTPClientGenerateStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"content\":\"hello\",\"stop\":false}\n{\"content\":\"\",\"stop\":true}\n"))
	}))
	defer server.Close()

	ch, err := NewHTTPClient().GenerateStream(context.Background(), server.URL, GenerateRequest{Model: "m", Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for chunk := range ch {
		if chunk.Error != "" {
			t.Fatal(chunk.Error)
		}
		parts = append(parts, chunk.Response)
		if chunk.Done {
			break
		}
	}
	if strings.Join(parts, "") != "hello" {
		t.Fatalf("stream parts = %#v, want hello", parts)
	}
}

func TestHTTPClientChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"chat ok"}}]}`))
	}))
	defer server.Close()

	resp, err := NewHTTPClient().Chat(context.Background(), server.URL, ChatRequest{
		Model:    "m",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "chat ok" {
		t.Fatalf("Chat() = %#v", resp)
	}
}
