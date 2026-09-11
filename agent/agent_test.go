package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"patcode/llm"
	"patcode/session"
	"patcode/tools"
)

type fakeProvider struct {
	mu     sync.Mutex
	calls  int
	turn1  []llm.StreamEvent
	turnN  []llm.StreamEvent
	always bool
}

func (f *fakeProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *fakeProvider) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()

	var events []llm.StreamEvent
	if n == 1 {
		events = f.turn1
	} else if f.always {
		events = f.turnN
	}

	ch := make(chan llm.StreamEvent)
	go func() {
		defer close(ch)
		for _, ev := range events {
			select {
			case <-ctx.Done():
				return
			case ch <- ev:
			}
		}
	}()
	return ch, nil
}

func toolCallEvent(t *testing.T, idx int, id, name, args string) llm.StreamEvent {
	t.Helper()
	return llm.StreamEvent{
		Type: llm.StreamToolCall,
		ToolCall: &llm.ToolCall{
			Index: idx,
			ID:    id,
			Type:  "function",
			Function: llm.ToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		},
	}
}

func TestAgentParallelToolExecutionKeepsOrder(t *testing.T) {
	// "read" tool that sleeps a duration encoded in args; two calls issued
	// together, the second sleeping longer than expected so it finishes
	// last. Results must still be stored in call order.
	var completionOrder []string
	var mu sync.Mutex

	readTool := tools.Tool{
		Name: "read",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *tools.ToolResult {
			var in struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return &tools.ToolResult{Success: false, Error: err.Error()}
			}
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			completionOrder = append(completionOrder, in.Path)
			mu.Unlock()
			return &tools.ToolResult{Success: true, Data: "content of " + in.Path}
		},
	}

	registry := tools.NewRegistry()
	registry.Register(readTool)
	registry.SetMode("build")

	ref := &fakeProvider{
		turn1: []llm.StreamEvent{
			toolCallEvent(t, 0, "call-1", "read", `{"path":"first.txt"}`),
			toolCallEvent(t, 1, "call-2", "read", `{"path":"second.txt"}`),
			{Type: llm.StreamDone, Done: true},
		},
		turnN: []llm.StreamEvent{
			{Type: llm.StreamChunk, Content: "done"},
			{Type: llm.StreamDone, Done: true},
		},
	}

	sess := session.New("/tmp", t.TempDir())
	sess.SetMode(session.ModeBuild)
	ag := New(ref, registry, sess, "test", 4)

	events := make(chan AgentEvent)
	go ag.Process(context.Background(), "run", events)
	for range events {
	}

	if len(completionOrder) != 2 {
		t.Fatalf("expected 2 tool completions, got %d", len(completionOrder))
	}
	if completionOrder[0] != "first.txt" || completionOrder[1] != "second.txt" {
		t.Errorf("unexpected completion order: %v", completionOrder)
	}

	var toolContents []string
	for _, m := range sess.GetMessages() {
		if m.Role == llm.RoleTool {
			toolContents = append(toolContents, m.Content)
		}
	}
	if len(toolContents) != 2 {
		t.Fatalf("expected 2 tool messages, got %d", len(toolContents))
	}
	if toolContents[0] != "content of first.txt" || toolContents[1] != "content of second.txt" {
		t.Errorf("unexpected tool message order: %v", toolContents)
	}
}

func TestAgentMaxTurnsLimit(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.Tool{
		Name: "read",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *tools.ToolResult {
			return &tools.ToolResult{Success: true, Data: "x"}
		},
	})
	registry.SetMode("build")

	always := &fakeProvider{
		turn1: []llm.StreamEvent{
			toolCallEvent(t, 0, "c", "read", `{"path":"a"}`),
			{Type: llm.StreamDone, Done: true},
		},
		turnN: []llm.StreamEvent{
			toolCallEvent(t, 0, "c", "read", `{"path":"a"}`),
			{Type: llm.StreamDone, Done: true},
		},
		always: true,
	}

	sess := session.New("/tmp", t.TempDir())
	sess.SetMode(session.ModeBuild)
	ag := New(always, registry, sess, "test", 3)

	events := make(chan AgentEvent)
	go ag.Process(context.Background(), "loop", events)
	for range events {
	}

	always.mu.Lock()
	defer always.mu.Unlock()
	if always.calls > 4 {
		t.Errorf("expected the agent loop to cap at maxTurns (3), made %d provider calls", always.calls)
	}
	if always.calls < 2 {
		t.Errorf("expected at least a couple of rounds, got %d", always.calls)
	}
}

func TestAgentToolDeniedEmitsToolResult(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.Tool{
		Name: "write",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string"},
				"content":   map[string]any{"type": "string"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *tools.ToolResult {
			return &tools.ToolResult{Success: true, Data: "should not happen"}
		},
	})
	registry.SetMode("inspect") // write is denied here

	results := make(chan string, 10)
	var count int32
	readTool := tools.Tool{
		Name:        "read",
		InputSchema: map[string]any{"type": "object"},
		Execute: func(ctx context.Context, args json.RawMessage) *tools.ToolResult {
			atomic.AddInt32(&count, 1)
			return &tools.ToolResult{Success: true, Data: "ok"}
		},
	}
	registry.Register(readTool)

	ref := &fakeProvider{
		turn1: []llm.StreamEvent{
			toolCallEvent(t, 0, "c1", "write", `{"file_path":"x","content":"y"}`),
			{Type: llm.StreamDone, Done: true},
		},
		turnN: []llm.StreamEvent{
			{Type: llm.StreamChunk, Content: "fin"},
			{Type: llm.StreamDone, Done: true},
		},
	}

	sess := session.New("/tmp", t.TempDir())
	sess.SetMode(session.ModeInspect)
	ag := New(ref, registry, sess, "test", 4)

	events := make(chan AgentEvent)
	go ag.Process(context.Background(), "go", events)
	for ev := range events {
		if ev.Type == EventToolResult {
			results <- ev.Content
		}
	}
	close(results)

	sawDenied := false
	for r := range results {
		if len(r) > 0 && r[0] == 'E' {
			sawDenied = true
		}
	}
	if !sawDenied {
		t.Error("expected a denied write tool to surface an error tool result")
	}
	if atomic.LoadInt32(&count) != 0 {
		t.Errorf("denied tool should not execute; read executed %d times", count)
	}
}
