package tools

import (
	"context"
	"encoding/json"
	"fmt"
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
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
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
	return tool.Execute(ctx, args)
}
