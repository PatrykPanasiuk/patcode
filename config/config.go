package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderOpenRouter Provider = "openrouter"
	ProviderAnthropic  Provider = "anthropic"
	ProviderOllama     Provider = "ollama"
	ProviderLocal      Provider = "local"
	ProviderBuiltin    Provider = "builtin"
)

type Config struct {
	Provider    Provider       `yaml:"provider"`
	APIKey      string         `yaml:"api_key,omitempty"`
	Model       string         `yaml:"model"`
	ModelPath   string         `yaml:"model_path,omitempty"`
	BaseURL     string         `yaml:"base_url,omitempty"`
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

func (c *Config) OllamaBaseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "http://localhost:11434/v1"
}

// LoadWithFallback tries projectDir/patcode.yaml first,
// then ~/.patcode/patcode.yaml, then returns DefaultConfig.
func LoadWithFallback(projectDir string) (*Config, string, error) {
	// Try project-level
	projectPath := filepath.Join(projectDir, "patcode.yaml")
	cfg, err := loadFile(projectPath)
	if err == nil {
		return cfg, projectPath, nil
	}
	if !os.IsNotExist(err) {
		return nil, "", err
	}

	// Try user-level
	home, _ := os.UserHomeDir()
	userPath := filepath.Join(home, ".patcode", "patcode.yaml")
	cfg, err = loadFile(userPath)
	if err == nil {
		return cfg, userPath, nil
	}
	if !os.IsNotExist(err) {
		return nil, "", err
	}

	return DefaultConfig(), "", nil
}

// loadFile loads config only if the file exists. Returns os.ErrNotExist if missing.
func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	expandHome := func(s string) string {
		if strings.HasPrefix(s, "~/") {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, s[2:])
		}
		return s
	}
	cfg.SessionDir = expandHome(cfg.SessionDir)
	cfg.ModelPath = expandHome(cfg.ModelPath)
	cfg.BaseURL = expandHome(cfg.BaseURL)
	cfg.EnsureEnvLoaded()
	return cfg, nil
}

func Load(path string) (*Config, error) {
	cfg, err := loadFile(path)
	if err == nil {
		return cfg, nil
	}
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	return nil, fmt.Errorf("reading config: %w", err)
}

func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	c := &Config{
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
	c.EnsureEnvLoaded()
	return c
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
