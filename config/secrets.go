package config

import (
	"os"
	"strings"
)

// SupportedEnvVars maps a provider type to the environment variable(s)
// that can supply its API key, in priority order. Environment variables
// take precedence over any api_key stored in a config file, so secrets
// never have to live in a file that could be committed to a repository.
var providerEnvVars = map[string][]string{
	"openai":     {"PATCODE_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY"},
	"openrouter": {"PATCODE_API_KEY", "OPENROUTER_API_KEY", "OPENAI_API_KEY"},
	"anthropic":  {"PATCODE_API_KEY", "ANTHROPIC_API_KEY"},
}

// ResolveAPIKey returns the effective API key for the given provider.
// It checks, in order:
//
//	1. provider-specific environment variables
//	2. the generic PATCODE_API_KEY
//	3. the value from the config file (v)
//
// A non-empty environment value always wins over the config file so that
// secrets can be supplied without writing them to disk.
func ResolveAPIKey(provider string, v string) string {
	if vars, ok := providerEnvVars[provider]; ok {
		for _, name := range vars {
			if val := strings.TrimSpace(os.Getenv(name)); val != "" {
				return val
			}
		}
	}
	return v
}

// EnsureEnvLoaded applies ResolveAPIKey to a config's APIKey field in place.
func (c *Config) EnsureEnvLoaded() {
	c.APIKey = ResolveAPIKey(string(c.Provider), c.APIKey)
}
