package agent

import (
	"context"
	"encoding/json"
	"fmt"
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
	Type   EventType `json:"type"`
	Content string   `json:"content,omitempty"`
	Tool   string    `json:"tool,omitempty"`
	Error  string    `json:"error,omitempty"`
}

type Agent struct {
	provider llm.Provider
	registry *tools.Registry
	session  *session.Session
	model    string
	mu       sync.Mutex
}

func New(provider llm.Provider, registry *tools.Registry, sess *session.Session, model string) *Agent {
	return &Agent{
		provider: provider,
		registry: registry,
		session:  sess,
		model:    model,
	}
}

func (a *Agent) Process(ctx context.Context, userMessage string, events chan<- AgentEvent) {
	defer close(events)

	a.session.AddMessage(llm.Message{
		Role:    llm.RoleUser,
		Content: userMessage,
	})

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
		Model:     a.model,
		Messages:  a.session.GetMessages(),
		System:    systemPrompt,
		Stream:    true,
	}

	if a.session.GetMode() == session.ModeBuild {
		msg.Tools = a.registry.Definitions()
	}

	stream, err := a.provider.ChatStream(ctx, msg)
	if err != nil {
		events <- AgentEvent{
			Type:  EventError,
			Error: fmt.Sprintf("failed to start chat: %v", err),
		}
		return
	}

	var responseContent string
	var toolCalls []llm.ToolCall

	events <- AgentEvent{Type: EventThinking}

	for event := range stream {
		switch event.Type {
		case llm.StreamChunk:
			responseContent += event.Content
			events <- AgentEvent{
				Type:    EventChunk,
				Content: event.Content,
			}

		case llm.StreamToolCall:
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, *event.ToolCall)
			}

		case llm.StreamDone:
			if responseContent != "" {
				a.session.AddMessage(llm.Message{
					Role:    llm.RoleAssistant,
					Content: responseContent,
				})
			}

			if len(toolCalls) > 0 {
				assistantMsg := llm.Message{
					Role:      llm.RoleAssistant,
					Content:   responseContent,
					ToolCalls: toolCalls,
				}
				a.session.AddMessage(assistantMsg)

				for _, tc := range toolCalls {
					events <- AgentEvent{
						Type:    EventToolCall,
						Tool:    tc.Function.Name,
						Content: tc.Function.Arguments,
					}

					var args json.RawMessage
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						args = json.RawMessage(tc.Function.Arguments)
					}

					result := a.registry.Execute(ctx, tc.Function.Name, args)

					var resultContent string
					if result.Success {
						resultContent = result.Data
					} else {
						resultContent = fmt.Sprintf("Error: %s", result.Error)
					}

					a.session.AddMessage(llm.Message{
						Role:       llm.RoleTool,
						Content:    resultContent,
						ToolCallID: tc.ID,
						Name:       tc.Function.Name,
					})

					events <- AgentEvent{
						Type:    EventToolResult,
						Tool:    tc.Function.Name,
						Content: resultContent,
					}
				}

				events <- AgentEvent{Type: EventThinking}

				stream2, err := a.provider.ChatStream(ctx, llm.ChatRequest{
					Model:    a.model,
					Messages: a.session.GetMessages(),
					System:   systemPrompt,
					Stream:   true,
				})
				if err != nil {
					events <- AgentEvent{
						Type:  EventError,
						Error: fmt.Sprintf("follow-up failed: %v", err),
					}
					return
				}

				var followUpContent string
				for event := range stream2 {
					switch event.Type {
					case llm.StreamChunk:
						followUpContent += event.Content
						events <- AgentEvent{
							Type:    EventChunk,
							Content: event.Content,
						}
					case llm.StreamDone:
						if followUpContent != "" {
							a.session.AddMessage(llm.Message{
								Role:    llm.RoleAssistant,
								Content: followUpContent,
							})
						}
					}
				}
			}

			a.session.Save()
			events <- AgentEvent{Type: EventDone}

		case llm.StreamError:
			events <- AgentEvent{
				Type:  EventError,
				Error: fmt.Sprintf("stream error: %v", event.Error),
			}
			return
		}
	}
}

func modePrompt(mode session.Mode) string {
	switch mode {
	case session.ModePlan:
		return `Mode: PLAN.
Write only a short, concrete plan.
Do not make file changes.
Do not call tools.
Do not give generic acknowledgements.`
	case session.ModeBuild:
		return `Mode: BUILD.
You are an execution agent, not a chat bot.
If the request is clear, inspect files and make the change immediately.
If you need clarification, ask one short question only.
Never reply with generic acknowledgements like "Rozumiem" or "Mogę pomóc".
Prefer concrete actions over explanations.
Use tools to inspect, edit, write, or run commands.`
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
