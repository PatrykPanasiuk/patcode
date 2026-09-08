package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"patcode/agent"
	"patcode/session"
)

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(max(40, msg.Width-4))
		m.viewport.Width = max(40, msg.Width-4)
		m.viewport.Height = max(10, msg.Height-11)
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.loading && m.cancelFunc != nil {
				m.cancelFunc()
				m.loading = false
				m.messages = append(m.messages, chatMessage{
					Role:    "system",
					Content: "Request cancelled.",
				})
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyCtrlL:
			m.messages = nil
			return m, nil

		case tea.KeyEnter:
			if m.loading {
				return m, nil
			}

			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}

			m.textarea.Reset()

			if m.session.GetMode() == session.ModeShell {
				return m, m.runShell(input)
			}

			if strings.HasPrefix(input, "/") {
				cmd := m.handleSlashCommand(input)
				if cmd != nil {
					return m, cmd
				}
				return m, nil
			}

			return m, m.sendMessage(input)

		case tea.KeyTab:
			m.cycleMode()
			return m, nil

		case tea.KeyPgUp:
			m.viewport.HalfViewUp()
			return m, nil

		case tea.KeyPgDown:
			m.viewport.HalfViewDown()
			return m, nil

		case tea.KeyEscape:
			if m.showHelp {
				m.showHelp = false
			}
			if m.showCommands {
				m.showCommands = false
			}
			return m, nil
		}

	case eventMsg:
		event := msg.Event
		switch event.Type {
		case agent.EventChunk:
			if m.session.GetMode() == session.ModeShell {
				m.messages = append(m.messages, chatMessage{
					Role:    "system",
					Content: event.Content,
				})
				m.viewport.GotoBottom()
				break
			}
			m.appendToLastAssistant(event.Content)
			m.viewport.GotoBottom()

		case agent.EventThinking:
			m.messages = append(m.messages, chatMessage{
				Role:    "assistant",
				Content: "",
			})

		case agent.EventToolCall:
			m.loading = true
			m.messages = append(m.messages, chatMessage{
				Role:    "tool",
				Content: fmt.Sprintf("🔧 Using tool: %s(%s)", event.Tool, event.Content),
			})
			m.viewport.GotoBottom()

		case agent.EventToolResult:
			m.loading = true
			m.messages = append(m.messages, chatMessage{
				Role:    "tool",
				Content: fmt.Sprintf("✅ Tool %s completed:\n%s", event.Tool, truncate(event.Content, 500)),
			})
			m.viewport.GotoBottom()

		case agent.EventDone:
			m.loading = false
			m.viewport.GotoBottom()
			return m, nil

		case agent.EventError:
			m.loading = false
			m.messages = append(m.messages, chatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Error: %s", event.Error),
			})
			m.viewport.GotoBottom()
			return m, nil
		}
		cmds = append(cmds, listenForEvents(msg.Events))

	case agentDoneMsg:
		m.loading = false
		return m, nil

	case error:
		m.err = msg
		return m, nil

	case setupDoneMsg:
		if msg.err != nil {
			m.providerType = "builtin"
			m.providerModel = "setup failed"
			m.messages = append(m.messages, chatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Auto-setup failed: %s", msg.err),
			})
		} else {
			m.providerType = "ollama"
			m.providerModel = smallModel
			m.providerReady = true
			m.messages = append(m.messages, chatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Model ready: Ollama + %s", smallModel),
			})
		}
		m.viewport.GotoBottom()
		return m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) appendToLastAssistant(content string) {
	if len(m.messages) == 0 {
		return
	}
	last := &m.messages[len(m.messages)-1]
	if last.Role == "assistant" {
		last.Content += content
	}
}

func (m *model) expandFileReferences(input string) string {
	parts := strings.Split(input, " ")
	for i, part := range parts {
		if strings.HasPrefix(part, "@") {
			ref := part[1:]
			if info, err := os.Stat(ref); err == nil && !info.IsDir() {
				data, err := os.ReadFile(ref)
				if err == nil {
					parts[i] = fmt.Sprintf("(contents of %s:\n```\n%s\n```\n)", ref, string(data))
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func (m *model) handleAutoComplete() tea.Cmd {
	return nil
}

func (m *model) findFiles(partial string) []string {
	var matches []string
	entries, err := os.ReadDir(m.projectDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, partial) {
			if entry.IsDir() {
				name += "/"
			}
			matches = append(matches, name)
		}
	}
	return matches
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
