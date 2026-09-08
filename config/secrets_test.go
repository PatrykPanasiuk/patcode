package config

import "testing"

func TestResolveAPIKeyEnvWins(t *testing.T) {
	t.Setenv("PATCODE_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-envkey")
	got := ResolveAPIKey("openrouter", "sk-or-v1-filekey")
	if got != "sk-or-v1-envkey" {
		t.Fatalf("expected env key to win, got %q", got)
	}
}

func TestResolveAPIKeyFileFallback(t *testing.T) {
	t.Setenv("PATCODE_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	got := ResolveAPIKey("openrouter", "sk-or-v1-filekey")
	if got != "sk-or-v1-filekey" {
		t.Fatalf("expected file key fallback, got %q", got)
	}
}

func TestResolveAPIKeyProviderSpecific(t *testing.T) {
	t.Setenv("PATCODE_API_KEY", "sk-xxx")
	t.Setenv("OPENROUTER_API_KEY", "")
	got := ResolveAPIKey("openrouter", "file")
	if got != "sk-xxx" {
		t.Fatalf("expected PATCODE_API_KEY to work for openrouter, got %q", got)
	}
}