package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalProviderDelegatesToServer(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":"c1","object":"chat.completion",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hello from gguf"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":4,"completion_tokens":3}
		}`))
	}))
	defer srv.Close()

	p, err := NewLocalProvider("", WithServerURL(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer p.Close()

	resp, err := p.Chat(context.Background(), ChatRequest{
		Model:  "test",
		System: "you are a test",
		Messages: []Message{
			{Role: RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Message.Content != "hello from gguf" {
		t.Errorf("expected 'hello from gguf', got %q", resp.Message.Content)
	}

	if gotBody["model"] != "test" {
		t.Errorf("expected model 'test', got %v", gotBody["model"])
	}
	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %v", gotBody["messages"])
	}
}

func TestLocalProviderRejectsMissingModelAndServer(t *testing.T) {
	t.Setenv("PATCODE_LLAMA_SERVER_URL", "")
	t.Setenv("PATCODE_LLAMA_BIN", "does-not-exist-llama-server")

	if _, err := NewLocalProvider(""); err == nil {
		t.Fatal("expected error without model path or server URL")
	}
}

func TestLocalProviderStreams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, _ := w.(http.Flusher)
		w.Write([]byte("data: " + `{"choices":[{"delta":{"content":"part1"},"finish_reason":null}]}` + "\n\n"))
		fl.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
		fl.Flush()
	}))
	defer srv.Close()

	p, err := NewLocalProvider("", WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	var got string
	ch, err := p.ChatStream(context.Background(), ChatRequest{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	for ev := range ch {
		if ev.Type == StreamChunk {
			got += ev.Content
		}
	}
	if got != "part1" {
		t.Errorf("expected streamed 'part1', got %q", got)
	}
}
