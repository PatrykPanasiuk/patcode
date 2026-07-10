package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"patcode/agent"
	"patcode/config"
	"patcode/llm"
	"patcode/session"
	"patcode/tools"
)

type chatMessage struct {
	Role    string
	Content string
}

type model struct {
	projectDir string
	config     *config.Config
	session    *session.Session
	agent      *agent.Agent
	provider   llm.Provider

	messages    []chatMessage
	textarea    textarea.Model
	viewport    viewport.Model
	spinner     spinner.Model
	loading     bool
	showHelp    bool
	showCommands bool

	width  int
	height int

	err error

	cancelFunc context.CancelFunc
}

func initialModel(projectDir string) (*model, error) {
	absDir, err := config.ResolveProjectDir(projectDir)
	if err != nil {
		return nil, fmt.Errorf("resolving project directory: %w", err)
	}

	cfgPath := filepath.Join(absDir, "patcode.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	home, _ := os.UserHomeDir()
	saveDir := cfg.SessionDir
	if saveDir == "" {
		saveDir = filepath.Join(home, ".patcode", "sessions")
	}

	sess := session.New(absDir, saveDir)

	provider, err := llm.NewProvider(string(cfg.Provider), cfg.APIKey, cfg.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("creating provider: %w", err)
	}

	registry := tools.DefaultRegistry(absDir)
	ag := agent.New(provider, registry, sess, cfg.Model)

	ti := textarea.New()
	ti.Placeholder = "message... (/? for help)"
	ti.Focus()
	ti.SetWidth(80)
	ti.SetHeight(1)
	ti.ShowLineNumbers = false
	ti.CharLimit = 0

	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = dimStyle

	return &model{
		projectDir:  absDir,
		config:      cfg,
		session:     sess,
		agent:       ag,
		provider:    provider,
		messages:    []chatMessage{},
		textarea: ti,
		viewport: vp,
		spinner:  s,
	}, nil
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

func (m *model) sendMessage(msg string) tea.Cmd {
	m.loading = true
	m.messages = append(m.messages, chatMessage{Role: "user", Content: msg})

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	events := make(chan agent.AgentEvent)
	go m.agent.Process(ctx, msg, events)

	return listenForEvents(events)
}

func (m *model) runShell(command string) tea.Cmd {
	m.loading = true
	m.messages = append(m.messages, chatMessage{Role: "user", Content: command})

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	events := make(chan agent.AgentEvent, 1)
	go func() {
		defer close(events)
		events <- agent.AgentEvent{Type: agent.EventThinking}
		cmd := exec.CommandContext(ctx, "bash", "-lc", command)
		cmd.Dir = m.projectDir
		out, err := cmd.CombinedOutput()
		if len(out) > 0 {
			events <- agent.AgentEvent{Type: agent.EventChunk, Content: string(out)}
		}
		if err != nil {
			events <- agent.AgentEvent{Type: agent.EventError, Error: err.Error()}
			return
		}
		events <- agent.AgentEvent{Type: agent.EventDone}
	}()

	return listenForEvents(events)
}

func (m *model) cycleMode() {
	switch m.session.GetMode() {
	case session.ModePlan:
		m.session.SetMode(session.ModeAsk)
	case session.ModeAsk:
		m.session.SetMode(session.ModeBuild)
	case session.ModeBuild:
		m.session.SetMode(session.ModeShell)
	default:
		m.session.SetMode(session.ModePlan)
	}
}

func (m *model) currentModeLabel() string {
	return string(m.session.GetMode())
}

func listenForEvents(events chan agent.AgentEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return agentDoneMsg{}
		}
		return eventMsg{Event: event, Events: events}
	}
}

type eventMsg struct {
	Event  agent.AgentEvent
	Events chan agent.AgentEvent
}

type agentDoneMsg struct{}

func (m *model) handleSlashCommand(cmd string) tea.Cmd {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}

	switch parts[0] {
	case "/help", "/?":
		m.showHelp = !m.showHelp
		if m.showHelp {
			m.messages = append(m.messages, chatMessage{
				Role: "system",
				Content: `patcode commands:
  /help, /?   toggle help
  /plan       plan mode (read-only)
  /ask        ask mode (answer only)
  /build      build mode (allow edits)
  /shell      shell mode (run commands)
  /mode       show mode
  /clear      clear chat
  /session    session info
  /exit       quit

shortcuts:
  Ctrl+C      cancel
  Ctrl+L      clear
  PgUp/PgDn   scroll
  Tab         cycle mode`,
			})
		}
		return nil

	case "/plan":
		m.session.SetMode(session.ModePlan)
		return nil

	case "/ask":
		m.session.SetMode(session.ModeAsk)
		return nil

	case "/build":
		m.session.SetMode(session.ModeBuild)
		return nil

	case "/shell":
		m.session.SetMode(session.ModeShell)
		return nil

	case "/mode":
		m.messages = append(m.messages, chatMessage{
			Role:    "system",
			Content: fmt.Sprintf("Current mode: %s", m.session.GetMode()),
		})
		return nil

	case "/clear":
		m.messages = []chatMessage{
			{Role: "system", Content: fmt.Sprintf("patcode — %s", filepath.Base(m.projectDir))},
		}
		return nil

	case "/session":
		m.messages = append(m.messages, chatMessage{
			Role:    "system",
			Content: fmt.Sprintf("Session ID: %s\nCreated: %s\nMessages: %d\nMode: %s",
				m.session.ID,
				m.session.CreatedAt.Format("2006-01-02 15:04:05"),
				len(m.messages),
				m.session.GetMode()),
		})
		return nil

	case "/project":
		m.messages = append(m.messages, chatMessage{
			Role:    "system",
			Content: fmt.Sprintf("Project directory: %s", m.projectDir),
		})
		return nil

	case "/commands":
		m.showCommands = !m.showCommands
		if m.showCommands && len(m.config.Commands) > 0 {
			var sb strings.Builder
			sb.WriteString("Custom commands:\n")
			for _, c := range m.config.Commands {
				sb.WriteString(fmt.Sprintf("  /%s - %s\n", c.Name, c.Description))
			}
			m.messages = append(m.messages, chatMessage{Role: "system", Content: sb.String()})
		} else if m.showCommands {
			m.messages = append(m.messages, chatMessage{Role: "system", Content: "No custom commands defined."})
		}
		return nil

	case "/exit":
		return tea.Quit

	default:
		for _, c := range m.config.Commands {
			if c.Name == parts[0][1:] {
				expanded := c.Prompt
				for i, arg := range parts[1:] {
					expanded = strings.ReplaceAll(expanded, fmt.Sprintf("{%d}", i), arg)
				}
				return m.sendMessage(expanded)
			}
		}
		m.messages = append(m.messages, chatMessage{
			Role:    "system",
			Content: fmt.Sprintf("unknown: %s (/? for help)", parts[0]),
		})
		return nil
	}
}
