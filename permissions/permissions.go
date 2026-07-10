package permissions

import (
	"fmt"
	"strings"
)

type Action string

const (
	ActionBash  Action = "bash"
	ActionRead  Action = "read"
	ActionWrite Action = "write"
	ActionEdit  Action = "edit"
	ActionGrep  Action = "grep"
	ActionGlob  Action = "glob"
)

type Mode string

const (
	ModeAllow Mode = "allow"
	ModeDeny  Mode = "deny"
	ModeAsk   Mode = "ask"
)

type Manager struct {
	autoApprove map[Action]bool
	defaultDeny map[Action]bool
}

func NewManager(autoApprove, defaultDeny []Action) *Manager {
	m := &Manager{
		autoApprove: make(map[Action]bool),
		defaultDeny: make(map[Action]bool),
	}
	for _, a := range autoApprove {
		m.autoApprove[a] = true
	}
	for _, a := range defaultDeny {
		m.defaultDeny[a] = true
	}
	return m
}

func (m *Manager) Check(action Action) Mode {
	if m.autoApprove[action] {
		return ModeAllow
	}
	if m.defaultDeny[action] {
		return ModeDeny
	}
	return ModeAsk
}

func ParseActions(actions []string) ([]Action, error) {
	result := make([]Action, len(actions))
	for i, a := range actions {
		switch strings.ToLower(a) {
		case "bash":
			result[i] = ActionBash
		case "read":
			result[i] = ActionRead
		case "write":
			result[i] = ActionWrite
		case "edit":
			result[i] = ActionEdit
		case "grep":
			result[i] = ActionGrep
		case "glob":
			result[i] = ActionGlob
		default:
			return nil, fmt.Errorf("unknown action: %s", a)
		}
	}
	return result, nil
}
