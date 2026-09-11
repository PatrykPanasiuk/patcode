package llm

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestNewProviderBuiltin(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Type: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	_, ok := p.(*BuiltinProvider)
	if !ok {
		t.Errorf("expected *BuiltinProvider, got %T", p)
	}
}

func TestNewProviderOpenAI(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Type:   "openai",
		APIKey: "sk-test",
		Model:  "gpt-4o",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, ok := p.(*OpenAIProvider)
	if !ok {
		t.Errorf("expected *OpenAIProvider, got %T", p)
	}
}

func TestNewProviderOpenAIWithBaseURL(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Type:    "openai",
		APIKey:  "sk-test",
		BaseURL: "https://custom.example.com/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	oai, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected *OpenAIProvider")
	}
	if oai.baseURL != "https://custom.example.com/v1" {
		t.Errorf("expected custom base URL, got %s", oai.baseURL)
	}
}

func TestNewProviderOpenAIFallbackModelPath(t *testing.T) {
	// Legacy: if BaseURL is empty but ModelPath is set, use ModelPath as base URL
	p, err := NewProvider(ProviderConfig{
		Type:      "openai",
		APIKey:    "sk-test",
		ModelPath: "https://legacy.example.com/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	oai, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected *OpenAIProvider")
	}
	if oai.baseURL != "https://legacy.example.com/v1" {
		t.Errorf("expected legacy base URL from model_path, got %s", oai.baseURL)
	}
}

func TestNewProviderOllama(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Type:  "ollama",
		Model: "llama3",
	})
	if err != nil {
		t.Fatal(err)
	}
	oai, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected *OpenAIProvider")
	}
	// Default Ollama URL
	if oai.baseURL != "http://localhost:11434/v1" {
		t.Errorf("expected default ollama URL, got %s", oai.baseURL)
	}
	// Should have empty api key
	if oai.apiKey != "" {
		t.Errorf("expected empty api key for ollama, got %s", oai.apiKey)
	}
}

func TestNewProviderOllamaWithBaseURL(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Type:    "ollama",
		BaseURL: "http://192.168.1.50:11434/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	oai, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected *OpenAIProvider")
	}
	if oai.baseURL != "http://192.168.1.50:11434/v1" {
		t.Errorf("expected custom ollama URL, got %s", oai.baseURL)
	}
}

func TestNewProviderAnthropic(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Type:   "anthropic",
		APIKey: "sk-ant-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, ok := p.(*AnthropicProvider)
	if !ok {
		t.Errorf("expected *AnthropicProvider, got %T", p)
	}
}

func TestNewProviderLocal(t *testing.T) {
	// With a server URL set, the local provider attaches to it instead of
	// attempting to spawn llama-server.
	t.Setenv("PATCODE_LLAMA_SERVER_URL", "http://127.0.0.1:19999/v1")
	p, err := NewProvider(ProviderConfig{
		Type:      "local",
		ModelPath: "/tmp/test.gguf",
	})
	if err != nil {
		t.Fatal(err)
	}
	lp, ok := p.(*LocalProvider)
	if !ok {
		t.Fatalf("expected *LocalProvider, got %T", p)
	}
	if lp.ServerURL() != "http://127.0.0.1:19999/v1" {
		t.Errorf("unexpected server url: %s", lp.ServerURL())
	}
}

func TestNewProviderLocalNoPath(t *testing.T) {
	t.Setenv("PATCODE_LLAMA_SERVER_URL", "")
	t.Setenv("PATCODE_LLAMA_BIN", "llama-server-that-does-not-exist")
	_, err := NewProvider(ProviderConfig{
		Type: "local",
	})
	if err == nil {
		t.Fatal("expected error for local provider without model path")
	}
	if !strings.Contains(err.Error(), "requires either") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewProviderUnknown(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Type: "nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	_, ok := p.(*BuiltinProvider)
	if !ok {
		t.Errorf("expected fallback to *BuiltinProvider, got %T", p)
	}
}

func TestBuiltinProviderChatStream(t *testing.T) {
	p := NewBuiltinProvider()
	ch, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for ev := range ch {
		if ev.Type == StreamChunk {
			got += ev.Content
		}
	}
	if !strings.Contains(got, "cześć") && !strings.Contains(got, "Cześć") {
		t.Errorf("expected greeting in builtin response, got %q", got)
	}
}

func TestOllamaTroubleshootHint(t *testing.T) {
	hint := ollamaTroubleshootHint(fmt.Errorf("connection refused"))
	if !strings.Contains(hint, "ollama serve") {
		t.Errorf("expected ollama serve hint, got %q", hint)
	}

	hint = ollamaTroubleshootHint(fmt.Errorf("no such host"))
	if !strings.Contains(hint, "ollama serve") {
		t.Errorf("expected ollama serve hint for DNS error, got %q", hint)
	}

	hint = ollamaTroubleshootHint(fmt.Errorf("timeout"))
	if !strings.Contains(hint, "timed out") {
		t.Errorf("expected timeout hint, got %q", hint)
	}

	hint = ollamaTroubleshootHint(fmt.Errorf("unexpected EOF"))
	if !strings.Contains(hint, "Connection error") {
		t.Errorf("expected generic connection error, got %q", hint)
	}
}
