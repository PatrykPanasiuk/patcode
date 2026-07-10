package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderOllama    Provider = "ollama"
	ProviderLocal     Provider = "local"
	ProviderBuiltin   Provider = "builtin"
)

type Config struct {
	Provider    Provider       `yaml:"provider"`
	APIKey      string         `yaml:"api_key,omitempty"`
	Model       string         `yaml:"model"`
	ModelPath   string         `yaml:"model_path,omitempty"`
	Temperature float64        `yaml:"temperature"`
	MaxTokens   int            `yaml:"max_tokens"`
	Permissions Permissions    `yaml:"permissions"`
	Theme       Theme          `yaml:"theme"`
	SessionDir  string         `yaml:"session_dir"`
	Agents      []AgentConfig  `yaml:"agents,omitempty"`
	Commands    []CustomCommand `yaml:"commands,omitempty"`
}

type Permissions struct {
	AutoApprove []string `yaml:"auto_approve,omitempty"`
	DefaultDeny []string `yaml:"default_deny,omitempty"`
}

type Theme struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
	Success   string `yaml:"success"`
	Warning   string `yaml:"warning"`
	Error     string `yaml:"error"`
}

type AgentConfig struct {
	Name        string   `yaml:"name"`
	Model       string   `yaml:"model"`
	SystemPrompt string  `yaml:"system_prompt"`
	Permissions []string `yaml:"permissions,omitempty"`
}

type CustomCommand struct {
	Name        string `yaml:"name"`
	Prompt      string `yaml:"prompt"`
	Description string `yaml:"description,omitempty"`
}

func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Provider:    ProviderBuiltin,
		Model:       "raczek",
		Temperature: 0.7,
		MaxTokens:   4096,
		SessionDir:  filepath.Join(home, ".patcode", "sessions"),
		Permissions: Permissions{
			AutoApprove: []string{"read", "glob", "grep"},
		},
		Theme: Theme{
			Primary:   "#888888",
			Secondary: "#aaaaaa",
			Success:   "#88aa88",
			Warning:   "#aaaa88",
			Error:     "#aa8888",
		},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
