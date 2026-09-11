package tui

import (
	"context"
	_ "embed"
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

	messages     []chatMessage
	textarea     textarea.Model
	viewport     viewport.Model
	spinner      spinner.Model
	loading      bool
	showHelp     bool
	showCommands bool

	width  int
	height int

	err error

	cancelFunc context.CancelFunc

	providerType  string
	providerModel string
	providerReady bool
}

const asciiArt = `.xX:$:
   +;      X;       .   .+:.             :x$.          +XX:                .:+:
   :;        :$.  x   $:x.   ;$x::+$;.;$    x; x+;:.  +    .X+.     .:;XX:   .&$
    X.    .     x$.    $X           :&      ;&&     .X$.       ;X: X.        X&+
    .$.   &&:    x. .  .&.  .       .X  .;  x&:       x&$.       .Xx;  :&&X+$&&.
     ;;   ;.    :;  $x  .&&&&  :&X;xx  .&&&&&;   X&;   ;&+  :&;    :x  .:. $&;:
     .$     .+$&x   :    .&.x   X&xx   X&+ .x   .&&+;   .x   &&$    +   .:+&&::
    .;&.  .&&&XX    .+;   .&X   .&X    +$+X;x;   $x:+   ;&   ;X.   ;;  &&&&;. :&.
    X  .   ;&.x:   x&&&.    +    $X          .+        :&X.       .x           ;&.
   .x       &$x   .&&+.   ;&+    .&&&X.       .X      .&.      .$&&:          :&&X
    ;&&x    $&:&&&X&X.&X&&&&x&&x.$&&.;X&&$: ;&&&&&&&$x&&.   +&&&&+.$&&&&$;  :&&&+
      .;&&&&&$.  .;$:  .X;.   .:&&x.     .+&&&:    .:;$;+&&&&&:       .:;X&&&&;
          ..                                              ..`

func initialModel(projectDir string, cfg *config.Config, sess *session.Session) (*model, error) {
	absDir, err := config.ResolveProjectDir(projectDir)
	if err != nil {
		return nil, fmt.Errorf("resolving project directory: %w", err)
	}

	home, _ := os.UserHomeDir()
	saveDir := cfg.SessionDir
	if saveDir == "" {
		saveDir = filepath.Join(home, ".patcode", "sessions")
	}

	if sess == nil {
		sess = session.New(absDir, saveDir)
	}
	sess.Project = absDir

	provider, err := llm.NewProvider(llm.ProviderConfig{
		Type:      string(cfg.Provider),
		APIKey:    cfg.APIKey,
		Model:     cfg.Model,
		ModelPath: cfg.ModelPath,
		BaseURL:   cfg.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("creating provider: %w", err)
	}

	registry := tools.DefaultRegistry(absDir)
	ag := agent.New(provider, registry, sess, cfg.Model, cfg.MaxTurns)

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
		projectDir:    absDir,
		config:        cfg,
		session:       sess,
		agent:         ag,
		provider:      provider,
		messages:      []chatMessage{},
		textarea:      ti,
		viewport:      vp,
		spinner:       s,
		providerType:  string(cfg.Provider),
		providerModel: cfg.Model,
		providerReady: string(cfg.Provider) != "builtin",
	}, nil
}

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, m.spinner.Tick}
	if !m.providerReady {
		m.messages = append(m.messages, chatMessage{
			Role:    "system",
			Content: "No AI model configured. Downloading the smallest model (smollm:135m via Ollama)...",
		})
		cmds = append(cmds, m.autoSetup())
	}
	return tea.Batch(cmds...)
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
	case session.ModeAsk:
		m.session.SetMode(session.ModeInspect)
	case session.ModeInspect:
		m.session.SetMode(session.ModePlan)
	case session.ModePlan:
		m.session.SetMode(session.ModeBuild)
	case session.ModeBuild:
		m.session.SetMode(session.ModeShell)
	default:
		m.session.SetMode(session.ModeAsk)
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
  /help, /?       toggle help
  /ask            ask mode (answer only)
  /inspect        inspect mode (read-only)
  /plan           plan mode (read-only)
  /review         review mode (read-only)
  /audit          audit mode (security review)
  /patch          patch mode (diff only)
  /build          build mode (allow edits)
  /fix            fix mode (fix failures)
  /refactor       refactor mode
  /scaffold       scaffold mode (create files)
  /test           test mode (run tests)
  /ci             ci mode (automation)
  /shell          shell mode (run commands)
  /mode           show mode
  /modes          list all modes
  /clear          clear chat
  /session        session info
  /exit           quit

shortcuts:
  Ctrl+C      cancel
  Ctrl+L      clear
  PgUp/PgDn   scroll
  Tab         cycle modes: ask -> inspect -> plan -> build -> shell`,
			})
		}
		return nil

	case "/ask":
		m.session.SetMode(session.ModeAsk)
		return nil

	case "/inspect":
		m.session.SetMode(session.ModeInspect)
		return nil

	case "/plan":
		m.session.SetMode(session.ModePlan)
		return nil

	case "/review":
		m.session.SetMode(session.ModeReview)
		return nil

	case "/audit":
		m.session.SetMode(session.ModeAudit)
		return nil

	case "/patch":
		m.session.SetMode(session.ModePatch)
		return nil

	case "/build":
		m.session.SetMode(session.ModeBuild)
		return nil

	case "/fix":
		m.session.SetMode(session.ModeFix)
		return nil

	case "/refactor":
		m.session.SetMode(session.ModeRefactor)
		return nil

	case "/scaffold":
		m.session.SetMode(session.ModeScaffold)
		return nil

	case "/test":
		m.session.SetMode(session.ModeTest)
		return nil

	case "/ci":
		m.session.SetMode(session.ModeCI)
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

	case "/modes":
		var sb strings.Builder
		sb.WriteString("Available modes:\n")
		for _, m := range session.AllModes() {
			sb.WriteString(fmt.Sprintf("  /%-12s %s\n", string(m), modeDescription(m)))
		}
		m.messages = append(m.messages, chatMessage{Role: "system", Content: sb.String()})
		return nil

	case "/clear":
		m.messages = []chatMessage{
			{Role: "system", Content: fmt.Sprintf("patcode — %s", filepath.Base(m.projectDir))},
		}
		return nil

	case "/session":
		m.messages = append(m.messages, chatMessage{
			Role: "system",
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

func modeDescription(m session.Mode) string {
	switch m {
	case session.ModeAsk:
		return "General answers without tools"
	case session.ModeInspect:
		return "Read-only repository inspection"
	case session.ModePlan:
		return "Implementation planning"
	case session.ModeReview:
		return "Code/diff review"
	case session.ModeAudit:
		return "Security-focused review"
	case session.ModePatch:
		return "Generate diff without applying"
	case session.ModeBuild:
		return "Implement changes"
	case session.ModeFix:
		return "Fix failing tests/errors"
	case session.ModeRefactor:
		return "Behavior-preserving changes"
	case session.ModeScaffold:
		return "Create initial structure"
	case session.ModeTest:
		return "Run and explain tests/lints"
	case session.ModeCI:
		return "Automation-friendly checks"
	case session.ModeShell:
		return "Direct shell passthrough"
	default:
		return ""
	}
}
