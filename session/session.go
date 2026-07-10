package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"patcode/llm"
)

type Mode string

const (
	ModePlan  Mode = "plan"
	ModeAsk   Mode = "ask"
	ModeBuild Mode = "build"
	ModeShell Mode = "shell"
)

type Session struct {
	ID        string        `json:"id"`
	Project   string        `json:"project"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Mode      Mode          `json:"mode"`
	Messages  []llm.Message `json:"messages"`
	System    string        `json:"system_prompt"`

	mu       sync.RWMutex
	savePath string
}

func New(project, saveDir string) *Session {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	return &Session{
		ID:        id,
		Project:   project,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Mode:      ModeAsk,
		Messages:  []llm.Message{},
		System:    defaultSystemPrompt(),
		savePath:  filepath.Join(saveDir, id+".json"),
	}
}

func defaultSystemPrompt() string {
	return `You are patcode, an open-source AI coding agent. You can help users write, debug, and refactor code.

Available tools:
- bash: Execute shell commands
- read: Read file contents
- write: Create or overwrite files
- edit: Make precise edits to files
- grep: Search file contents with regex
- glob: Find files matching glob patterns

When in PLAN mode, you should only plan and discuss changes without making modifications.
When in ASK mode, answer the user's question directly and do not propose or make changes unless asked.
When in BUILD mode, you must act like an execution agent: inspect files, make edits, and run commands when the request is clear. Do not answer with generic acknowledgements.
When in SHELL mode, the user is operating the shell directly and you should not use coding tools.

Always explain your reasoning before using tools.`
}

func (s *Session) AddMessage(msg llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

func (s *Session) GetMessages() []llm.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]llm.Message, len(s.Messages))
	copy(result, s.Messages)
	return result
}

func (s *Session) SetMode(mode Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Mode = mode
}

func (s *Session) GetMode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Mode
}

func (s *Session) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Dir(s.savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling session: %w", err)
	}

	if err := os.WriteFile(s.savePath, data, 0644); err != nil {
		return fmt.Errorf("writing session: %w", err)
	}

	return nil
}

func Load(id, saveDir string) (*Session, error) {
	path := filepath.Join(saveDir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading session: %w", err)
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing session: %w", err)
	}

	s.savePath = path
	return &s, nil
}

func ListSessions(saveDir string) ([]*Session, error) {
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions directory: %w", err)
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(saveDir, entry.Name()))
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		s.savePath = filepath.Join(saveDir, entry.Name())
		sessions = append(sessions, &s)
	}
	return sessions, nil
}
