package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"patcode/llm"
	"patcode/ragbridge"
	"patcode/session"
	"patcode/tools"
)

type EventType string

const (
	EventThinking   EventType = "thinking"
	EventChunk      EventType = "chunk"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventDone       EventType = "done"
	EventError      EventType = "error"
)

type AgentEvent struct {
	Type    EventType `json:"type"`
	Content string    `json:"content,omitempty"`
	Tool    string    `json:"tool,omitempty"`
	Error   string    `json:"error,omitempty"`
}

type Agent struct {
	provider llm.Provider
	registry *tools.Registry
	session  *session.Session
	model    string
	maxTurns int
	mu       sync.Mutex
}

const defaultMaxTurns = 8

func New(provider llm.Provider, registry *tools.Registry, sess *session.Session, model string, maxTurns int) *Agent {
	if maxTurns < 1 {
		maxTurns = defaultMaxTurns
	}
	return &Agent{
		provider: provider,
		registry: registry,
		session:  sess,
		model:    model,
		maxTurns: maxTurns,
	}
}

func (a *Agent) Process(ctx context.Context, userMessage string, events chan<- AgentEvent) {
	defer close(events)

	a.session.AddMessage(llm.Message{
		Role:    llm.RoleUser,
		Content: userMessage,
	})

	mode := string(a.session.GetMode())
	a.registry.SetMode(mode)

	systemPrompt := a.session.System + "\n\n" + modePrompt(a.session.GetMode())
	if cfg := ragbridge.DefaultConfig(a.session.Project); cfg.Enabled {
		if ragCtx, err := ragbridge.ContextForQuery(ctx, cfg, userMessage); err == nil && ragCtx != "" {
			systemPrompt = systemPrompt + ragCtx
		}
		memoryCfg := cfg
		memoryCfg.FilterProject = "patcode-memory"
		memoryCfg.MinQueryLen = 3
		if memoryCtx, err := ragbridge.ContextForQuery(ctx, memoryCfg, userMessage); err == nil && memoryCtx != "" {
			systemPrompt = systemPrompt + "\n\n=== PATCODE MEMORY ===\n" + memoryCtx
		}
	}

	msg := llm.ChatRequest{
		Model:    a.model,
		Messages: a.session.GetMessages(),
		System:   systemPrompt,
		Stream:   true,
	}

	{
		if mode != "shell" {
			msg.Tools = a.registry.DefinitionsForMode(mode)
		}
	}

	maxTurns := a.maxTurns
	turnFinished := false

	for turn := 0; turn < maxTurns && !turnFinished; turn++ {
		var responseContent string
		var toolCalls []llm.ToolCall

		events <- AgentEvent{Type: EventThinking}

		if mode != "shell" {
			msg.Tools = a.registry.DefinitionsForMode(mode)
		}

		stream, err := a.provider.ChatStream(ctx, msg)
		if err != nil {
			events <- AgentEvent{
				Type:  EventError,
				Error: fmt.Sprintf("failed to start chat: %v", err),
			}
			return
		}

		// Streaming tool calls arrive as fragments that must be merged by
		// index; each run builds a fresh map for the current turn.
		toolCallMap := map[int]*llm.ToolCall{}

		flushToolCalls := func() {
			keys := make([]int, 0, len(toolCallMap))
			for k := range toolCallMap {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			toolCalls = toolCalls[:0]
			for _, k := range keys {
				toolCalls = append(toolCalls, *toolCallMap[k])
			}
		}

		for event := range stream {
			switch event.Type {
			case llm.StreamChunk:
				responseContent += event.Content
				events <- AgentEvent{
					Type:    EventChunk,
					Content: event.Content,
				}

			case llm.StreamToolCall:
				if event.ToolCall == nil {
					continue
				}
				tc := event.ToolCall
				merged, ok := toolCallMap[tc.Index]
				if !ok {
					merged = &llm.ToolCall{Index: tc.Index, Type: "function"}
					toolCallMap[tc.Index] = merged
				}
				if tc.ID != "" {
					merged.ID = tc.ID
				}
				if tc.Function.Name != "" {
					merged.Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					merged.Function.Arguments += tc.Function.Arguments
				}

			case llm.StreamDone:
				flushToolCalls()

				assistantMsg := llm.Message{
					Role:    llm.RoleAssistant,
					Content: responseContent,
				}
				if len(toolCalls) > 0 {
					assistantMsg.ToolCalls = toolCalls
				}
				a.session.AddMessage(assistantMsg)

				if len(toolCalls) == 0 {
					turnFinished = true
					continue
				}

				// Announce all tool calls in order first so the UI stays ordered.
				for _, tc := range toolCalls {
					events <- AgentEvent{
						Type:    EventToolCall,
						Tool:    tc.Function.Name,
						Content: tc.Function.Arguments,
					}
				}

				// Execute all tool calls in parallel; results are stored by
				// index so session messages keep the original order.
				type toolOutcome struct {
					tc     llm.ToolCall
					result *tools.ToolResult
				}
				outcomes := make([]toolOutcome, len(toolCalls))
				var wg sync.WaitGroup
				for i, tc := range toolCalls {
					wg.Add(1)
					go func(i int, tc llm.ToolCall) {
						defer wg.Done()
						var args json.RawMessage
						if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
							args = json.RawMessage(tc.Function.Arguments)
						}
						// Tool execution failures are surfaced through the regular
						// ToolResult path; cancelled calls stop early.
						outcomes[i] = toolOutcome{tc: tc, result: a.registry.Execute(ctx, tc.Function.Name, args)}
					}(i, tc)
				}
				wg.Wait()

				for _, out := range outcomes {
					resultContent := out.result.Data
					if !out.result.Success {
						resultContent = fmt.Sprintf("Error: %s", out.result.Error)
					}

					a.session.AddMessage(llm.Message{
						Role:       llm.RoleTool,
						Content:    resultContent,
						ToolCallID: out.tc.ID,
						Name:       out.tc.Function.Name,
					})

					events <- AgentEvent{
						Type:    EventToolResult,
						Tool:    out.tc.Function.Name,
						Content: resultContent,
					}
				}
				// trap the case where the last tool call result should
				// feed another model round; fall through to next turn.

			case llm.StreamError:
				events <- AgentEvent{
					Type:  EventError,
					Error: fmt.Sprintf("stream error: %v", event.Error),
				}
				return
			}
		}

		// Build the next request from the accumulated conversation.
		msg = llm.ChatRequest{
			Model:    a.model,
			Messages: a.session.GetMessages(),
			System:   systemPrompt,
			Stream:   true,
		}
		if mode != "shell" {
			msg.Tools = a.registry.DefinitionsForMode(mode)
		}
	}

	a.session.Save()
	events <- AgentEvent{Type: EventDone}
}

func modePrompt(mode session.Mode) string {
	switch mode {
	case session.ModeAsk:
		return `Mode: ASK.
Answer the user's question directly and concisely.
Do not make file changes unless explicitly asked.
Do not give generic acknowledgements.`
	case session.ModeInspect:
		return `Mode: INSPECT.
Inspect the repository using read-only tools.
Summarize architecture, key files, risks, and entrypoints.
Do not modify files.`
	case session.ModePlan:
		return `Mode: PLAN.
Inspect files if useful.
Produce a concrete implementation plan with specific steps.
Do not modify files.
Do not give generic acknowledgements.`
	case session.ModeReview:
		return `Mode: REVIEW.
Review the code, repo, or diff.
Output findings with: severity, file/path, issue, and recommendation.
Do not modify files.`
	case session.ModeAudit:
		return `Mode: AUDIT.
Perform a security-focused review.
Focus on: auth, sessions, CSRF, SSRF, path traversal, command injection,
secrets, RBAC/IDOR, unsafe shell usage, dependency risks.
Include exploitability assessment and mitigation advice.
Do not modify files.`
	case session.ModePatch:
		return `Mode: PATCH.
Generate a unified diff only.
Do not claim files were changed.
Do not use write or edit tools.`
	case session.ModeBuild:
		return `Mode: BUILD.
You are an execution agent, not a chat bot.
If the request is clear, inspect files and make the change immediately.
If you need clarification, ask one short question only.
Never reply with generic acknowledgements.
Prefer concrete actions over explanations.
Use tools to inspect, edit, write, or run commands.`
	case session.ModeFix:
		return `Mode: FIX.
Diagnose failing tests or errors.
Modify minimal code to fix the issue.
Run relevant tests if allowed.
Summarize root cause and fix.`
	case session.ModeRefactor:
		return `Mode: REFACTOR.
Preserve existing behaviour.
Keep public API stable unless explicitly asked.
Run tests if allowed.
Explain risk of changes.`
	case session.ModeScaffold:
		return `Mode: SCAFFOLD.
Create minimal project structure.
Avoid overengineering.
Use existing project conventions.
Do not modify existing files unless necessary for setup.`
	case session.ModeTest:
		return `Mode: TEST.
Run allowed tests and lints.
Explain failures clearly.
Do not modify files.`
	case session.ModeCI:
		return `Mode: CI.
Concisely review and summarize.
Output should be automation-friendly.
Do not modify files.`
	case session.ModeShell:
		return `Mode: SHELL.
The user is operating the shell directly.
Do not call coding tools.`
	default:
		return `Mode: ASK.
Answer the user's question directly and concisely.
Do not make file changes unless explicitly asked.
Do not give generic acknowledgements.`
	}
}
