package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Provider != ProviderBuiltin {
		t.Errorf("expected builtin provider, got %s", cfg.Provider)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("expected 0.7, got %.1f", cfg.Temperature)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("expected 4096, got %d", cfg.MaxTokens)
	}
}

func TestOllamaBaseURL(t *testing.T) {
	cfg := DefaultConfig()
	// Without BaseURL set, should return default
	if got := cfg.OllamaBaseURL(); got != "http://localhost:11434/v1" {
		t.Errorf("expected default, got %s", got)
	}
	// With custom BaseURL
	cfg.BaseURL = "http://192.168.1.100:11434/v1"
	if got := cfg.OllamaBaseURL(); got != "http://192.168.1.100:11434/v1" {
		t.Errorf("expected custom URL, got %s", got)
	}
	// Trailing slash trimming
	cfg.BaseURL = "http://192.168.1.100:11434/v1/"
	if got := cfg.OllamaBaseURL(); got != "http://192.168.1.100:11434/v1" {
		t.Errorf("expected trimmed slash, got %s", got)
	}
}

func TestLoadExpandsHome(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
session_dir: ~/patcode-sessions
model_path: ~/models/test.gguf
base_url: ~/some-path
`
	yamlPath := filepath.Join(dir, "patcode.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(yamlPath)
	if err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(cfg.SessionDir, home) {
		t.Errorf("expected session_dir to start with home, got %s", cfg.SessionDir)
	}
	if !strings.HasPrefix(cfg.ModelPath, home) {
		t.Errorf("expected model_path to start with home, got %s", cfg.ModelPath)
	}
	if !strings.HasPrefix(cfg.BaseURL, home) {
		t.Errorf("expected base_url to start with home, got %s", cfg.BaseURL)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/patcode.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != ProviderBuiltin {
		t.Errorf("expected builtin default, got %s", cfg.Provider)
	}
}

func TestConfigSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "patcode.yaml")

	cfg := DefaultConfig()
	cfg.Provider = ProviderOllama
	cfg.Model = "llama3"
	cfg.Temperature = 0.5

	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Provider != ProviderOllama {
		t.Errorf("expected ollama, got %s", loaded.Provider)
	}
	if loaded.Model != "llama3" {
		t.Errorf("expected llama3, got %s", loaded.Model)
	}
	if loaded.Temperature != 0.5 {
		t.Errorf("expected 0.5, got %.1f", loaded.Temperature)
	}
}
