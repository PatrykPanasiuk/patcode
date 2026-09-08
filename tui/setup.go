package tui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"patcode/config"
)

const (
	smallModel = "smollm:135m"
	ollamaURL  = "https://ollama.com/download/ollama-linux-amd64"
)

type setupDoneMsg struct {
	err error
}

func (m *model) autoSetup() tea.Cmd {
	return func() tea.Msg {
		ollamaPath, err := exec.LookPath("ollama")
		if err != nil {
			ollamaPath = filepath.Join(os.Getenv("HOME"), ".local", "bin", "ollama")
			if _, err := os.Stat(ollamaPath); os.IsNotExist(err) {
				if err := downloadOllama(ollamaPath); err != nil {
					return setupDoneMsg{err: fmt.Errorf("downloading ollama: %w", err)}
				}
			}
		}

		if err := pullModel(ollamaPath); err != nil {
			return setupDoneMsg{err: fmt.Errorf("pulling model: %w", err)}
		}

		home, _ := os.UserHomeDir()
		cfgPath := filepath.Join(home, ".patcode", "patcode.yaml")
		cfg := config.DefaultConfig()
		cfg.Provider = config.Provider("ollama")
		cfg.Model = smallModel
		os.MkdirAll(filepath.Dir(cfgPath), 0755)
		cfg.Save(cfgPath)

		return setupDoneMsg{err: nil}
	}
}

func downloadOllama(path string) error {
	os.MkdirAll(filepath.Dir(path), 0755)

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(ollamaURL)
	if err != nil {
		return fmt.Errorf("downloading ollama binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("writing ollama binary: %w", err)
	}

	if err := os.Chmod(path, 0755); err != nil {
		return err
	}

	return nil
}

func pullModel(ollamaPath string) error {
	cmd := exec.Command(ollamaPath, "pull", smallModel)
	cmd.Env = append(os.Environ(), "OLLAMA_MODELS="+filepath.Join(os.Getenv("HOME"), ".ollama", "models"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ollama pull failed: %w", err)
	}
	return nil
}
