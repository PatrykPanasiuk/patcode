package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"patcode/config"
	"patcode/session"
)

func Run(projectDir string, cfg *config.Config) error {
	return RunWithSession(projectDir, cfg, nil)
}

func RunWithSession(projectDir string, cfg *config.Config, sess *session.Session) error {
	m, err := initialModel(projectDir, cfg, sess)
	if err != nil {
		return fmt.Errorf("initializing TUI: %w", err)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		return err
	}
	return nil
}
