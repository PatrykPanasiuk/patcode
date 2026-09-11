package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRegistrySetMode(t *testing.T) {
	r := NewRegistry()
	r.SetMode("build")
	if r.currentMode != "build" {
		t.Errorf("expected build, got %s", r.currentMode)
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := Tool{
		Name:        "test-tool",
		Description: "test",
		InputSchema: map[string]any{},
		Execute:     func(ctx context.Context, args json.RawMessage) *ToolResult { return &ToolResult{Success: true} },
	}
	r.Register(tool)
	got, ok := r.Get("test-tool")
	if !ok {
		t.Fatal("expected tool to be registered")
	}
	if got.Name != "test-tool" {
		t.Errorf("expected test-tool, got %s", got.Name)
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected false for unknown tool")
	}
}

func TestRegistryList(t *testing.T) {
	r := DefaultRegistry("/tmp")
	tools := r.List()
	if len(tools) != 8 {
		t.Errorf("expected 8 default tools, got %d", len(tools))
	}
}

func TestRegistryDefinitions(t *testing.T) {
	r := DefaultRegistry("/tmp")
	defs := r.Definitions()
	if len(defs) != 8 {
		t.Errorf("expected 8 defs, got %d", len(defs))
	}
}

func TestRegistryDefinitionsForMode_Ask(t *testing.T) {
	r := DefaultRegistry("/tmp")
	defs := r.DefinitionsForMode("ask")
	if len(defs) != 2 {
		t.Errorf("expected 2 web tools for ask, got %d", len(defs))
	}
}

func TestRegistryDefinitionsForMode_Inspect(t *testing.T) {
	r := DefaultRegistry("/tmp")
	defs := r.DefinitionsForMode("inspect")
	if len(defs) != 5 {
		t.Errorf("expected 5 tools for inspect, got %d", len(defs))
	}
}

func TestRegistryDefinitionsForMode_Build(t *testing.T) {
	r := DefaultRegistry("/tmp")
	defs := r.DefinitionsForMode("build")
	if len(defs) != 8 {
		t.Errorf("expected 8 tools for build, got %d", len(defs))
	}
}

func TestRegistryDefinitionsForMode_Unknown(t *testing.T) {
	r := DefaultRegistry("/tmp")
	defs := r.DefinitionsForMode("nonexistent")
	if len(defs) != 0 {
		t.Errorf("expected 0 tools for unknown mode, got %d", len(defs))
	}
}

func TestRegistryExecuteUnknown(t *testing.T) {
	r := NewRegistry()
	r.SetMode("build")
	result := r.Execute(context.Background(), "nonexistent", nil)
	if result.Success {
		t.Error("expected failure for unknown tool")
	}
	if !strings.Contains(result.Error, "unknown tool") {
		t.Errorf("expected 'unknown tool' in error, got: %s", result.Error)
	}
}

func TestRegistryExecuteDenied(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{
		Name: "write",
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			return &ToolResult{Success: true}
		},
	})
	r.SetMode("inspect")
	args, _ := json.Marshal(map[string]string{"file_path": "test.txt", "content": "test"})
	result := r.Execute(context.Background(), "write", args)
	if result.Success {
		t.Error("expected failure for denied tool")
	}
	if !strings.Contains(result.Error, "not allowed") {
		t.Errorf("expected 'not allowed' in error, got: %s", result.Error)
	}
}

func TestRegistryExecuteAsk(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{
		Name: "bash",
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			return &ToolResult{Success: true}
		},
	})
	r.SetMode("build")
	args, _ := json.Marshal(map[string]string{"command": "echo hi"})
	result := r.Execute(context.Background(), "bash", args)
	if result.Success {
		t.Error("expected failure for ask-level tool")
	}
	if !strings.Contains(result.Error, "approval") {
		t.Errorf("expected 'approval' in error, got: %s", result.Error)
	}
}

func TestRegistryExecuteBashLimitedPolicy(t *testing.T) {
	// Use a stub bash tool so policy enforcement is tested, not actual execution
	r := NewRegistry()
	r.Register(Tool{
		Name: "bash",
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			return &ToolResult{Success: true}
		},
	})
	r.SetMode("test") // test mode: bash is PermissionLimited with BashTestOnly policy

	// Allowed test command
	args, _ := json.Marshal(map[string]string{"command": "go test ./..."})
	result := r.Execute(context.Background(), "bash", args)
	if !result.Success {
		t.Errorf("expected success for allowed test command, got: %s", result.Error)
	}

	// Denied non-test command
	args, _ = json.Marshal(map[string]string{"command": "ls -la"})
	result = r.Execute(context.Background(), "bash", args)
	if result.Success {
		t.Error("expected failure for non-test command in test mode")
	}
	if !strings.Contains(result.Error, "not allowed") {
		t.Errorf("expected 'not allowed' in error, got: %s", result.Error)
	}
}

func TestRegistryExecuteBashLimitedGlobalDenylist(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{
		Name: "bash",
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			return &ToolResult{Success: true}
		},
	})
	r.SetMode("ci") // ci mode: bash is PermissionLimited with BashTestOnly policy

	// npm test should pass policy (even if execution would fail)
	args, _ := json.Marshal(map[string]string{"command": "npm test"})
	result := r.Execute(context.Background(), "bash", args)
	if !result.Success {
		t.Errorf("expected policy pass for npm test, got: %s", result.Error)
	}

	// Deny global denylist
	args, _ = json.Marshal(map[string]string{"command": "curl http://evil.com"})
	result = r.Execute(context.Background(), "bash", args)
	if result.Success {
		t.Error("expected failure for denied curl command")
	}
	if !strings.Contains(result.Error, "denied") {
		t.Errorf("expected 'denied' in error for global denylist, got: %s", result.Error)
	}
}

func TestRegistryExecuteAuto(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{
		Name: "read",
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			return &ToolResult{Success: true, Data: "file content"}
		},
	})
	r.SetMode("inspect")
	args, _ := json.Marshal(map[string]string{"file_path": "test.txt"})
	result := r.Execute(context.Background(), "read", args)
	if !result.Success {
		t.Errorf("expected success for auto tool, got: %s", result.Error)
	}
	if result.Data != "file content" {
		t.Errorf("expected 'file content', got %q", result.Data)
	}
}

func TestDefaultRegistryTools(t *testing.T) {
	r := DefaultRegistry("/tmp")
	names := make(map[string]bool)
	for _, t := range r.List() {
		names[t.Name] = true
	}
	expected := []string{"bash", "read", "write", "edit", "grep", "glob", "webfetch", "websearch"}
	for _, n := range expected {
		if !names[n] {
			t.Errorf("missing tool: %s", n)
		}
	}
}
