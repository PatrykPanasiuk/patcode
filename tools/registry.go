package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"patcode/permissions"
)

type ToolResult struct {
	Success bool            `json:"success"`
	Data    string          `json:"data"`
	Error   string          `json:"error,omitempty"`
	JSON    json.RawMessage `json:"json,omitempty"`
}

type ToolFunc func(ctx context.Context, args json.RawMessage) *ToolResult

type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

type Tool struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	InputSchema any      `json:"input_schema"`
	Execute     ToolFunc `json:"-"`
}

type Registry struct {
	tools        map[string]Tool
	currentMode  string
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) SetMode(mode string) {
	r.currentMode = mode
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

func (r *Registry) Definitions() []ToolDefinition {
	defs := make([]ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return defs
}

func DefaultRegistry(workdir string) *Registry {
	r := NewRegistry()
	r.Register(BashTool(workdir))
	r.Register(ReadTool(workdir))
	r.Register(WriteTool(workdir))
	r.Register(EditTool(workdir))
	r.Register(GrepTool(workdir))
	r.Register(GlobTool(workdir))
	return r
}

func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) *ToolResult {
	tool, ok := r.Get(name)
	if !ok {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("unknown tool: %s", name),
		}
	}

	perm := permissions.IsToolAllowed(r.currentMode, name)
	switch perm {
	case permissions.PermissionDeny:
		return &ToolResult{
			Success: false,
			Error:   permissions.ToolDeniedError(r.currentMode, name),
		}
	case permissions.PermissionAsk:
		return &ToolResult{
			Success: false,
			Error:   permissions.ToolAskError(r.currentMode, name),
		}
	case permissions.PermissionLimited:
		if name == "bash" {
			bashPolicy := permissions.BashPolicyForMode(r.currentMode)
			var bashArgs struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal(args, &bashArgs); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("invalid bash args: %v", err),
				}
			}
			if err := permissions.CheckBashCommand(bashArgs.Command, bashPolicy); err != nil {
				return &ToolResult{
					Success: false,
					Error:   err.Error(),
				}
			}
		}
		return tool.Execute(ctx, args)
	default:
		return tool.Execute(ctx, args)
	}
}

func (r *Registry) DefinitionsForMode(mode string) []ToolDefinition {
	allowed := permissions.AllowedToolNames(mode)
	allowedSet := make(map[string]bool, len(allowed))
	for _, n := range allowed {
		allowedSet[n] = true
	}
	defs := make([]ToolDefinition, 0, len(allowed))
	for _, t := range r.tools {
		if allowedSet[t.Name] {
			defs = append(defs, ToolDefinition{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}
	return defs
}
