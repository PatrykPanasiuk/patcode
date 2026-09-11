package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"patcode/tools"
)

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

type Message struct {
	Role       MessageRole `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	Name       string      `json:"name,omitempty"`
}

type ToolCall struct {
	Index    int              `json:"index,omitempty"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
}

type ChatRequest struct {
	Model       string                 `json:"model"`
	Messages    []Message              `json:"messages"`
	System      string                 `json:"system,omitempty"`
	Tools       []tools.ToolDefinition `json:"tools,omitempty"`
	Temperature float64                `json:"temperature"`
	MaxTokens   int                    `json:"max_tokens"`
	Stream      bool                   `json:"stream"`
}

type ChatResponse struct {
	Message Message `json:"message"`
	Usage   Usage   `json:"usage,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type StreamEvent struct {
	Type     StreamEventType `json:"type"`
	Content  string          `json:"content,omitempty"`
	ToolCall *ToolCall       `json:"tool_call,omitempty"`
	Done     bool            `json:"done,omitempty"`
	Error    error           `json:"error,omitempty"`
}

type StreamEventType string

const (
	StreamChunk    StreamEventType = "chunk"
	StreamToolCall StreamEventType = "tool_call"
	StreamDone     StreamEventType = "done"
	StreamError    StreamEventType = "error"
)

type ProviderConfig struct {
	Type      string
	APIKey    string
	Model     string
	ModelPath string
	BaseURL   string
}

func NewProvider(cfg ProviderConfig) (Provider, error) {
	switch cfg.Type {
	case "openai":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = cfg.ModelPath // fallback for legacy configs
		}
		return NewOpenAIProvider(cfg.APIKey, baseURL), nil
	case "openrouter":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = cfg.ModelPath // fallback for legacy configs
		}
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
		return NewOpenAIProvider(cfg.APIKey, baseURL), nil
	case "anthropic":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = cfg.ModelPath
		}
		return NewAnthropicProvider(cfg.APIKey, baseURL), nil
	case "ollama":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		p := NewOpenAIProvider("", baseURL)
		p.client = &http.Client{
			Transport: &ollamaErrorTransport{inner: http.DefaultTransport},
		}
		return p, nil
	case "local":
		return NewLocalProvider(cfg.ModelPath, WithServerURL(cfg.BaseURL))
	case "builtin":
		return NewBuiltinProvider(), nil
	default:
		return NewBuiltinProvider(), nil
	}
}

type ollamaErrorTransport struct {
	inner http.RoundTripper
}

func (t *ollamaErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot reach Ollama at %s\n\n%s",
			req.URL.Host,
			ollamaTroubleshootHint(err),
		)
	}
	return resp, nil
}

func ollamaTroubleshootHint(err error) string {
	s := err.Error()
	if strings.Contains(s, "connection refused") || strings.Contains(s, "no such host") {
		return "Ollama may not be running. Start it with:\n  ollama serve\n\n" +
			"Or install it from https://ollama.com/download\n\n" +
			"If Ollama runs on a custom host/port, set base_url in patcode.yaml."
	}
	if strings.Contains(s, "timeout") {
		return "Connection to Ollama timed out. Check that ollama serve is running."
	}
	return fmt.Sprintf("Connection error: %s", err)
}

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

type openaiChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	Tools       []openaiTool    `json:"tools,omitempty"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
	Stream      bool            `json:"stream"`
}

// openaiTool is the OpenAI-compatible tool definition format:
//
//	{"type":"function","function":{"name","description","parameters"}}
//
// The project's ToolDefinition uses the Anthropic "input_schema" shape, so
// tools must be converted to this format before being sent to OpenAI endpoints.
type openaiTool struct {
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}

type openaiToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// toOpenAITools converts the shared Anthropic-style ToolDefinitions into the
// OpenAI tool-call format required by OpenAI, OpenRouter, and Ollama's
// OpenAI-compatible endpoint.
func toOpenAITools(defs []tools.ToolDefinition) []openaiTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]openaiTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, openaiTool{
			Type: "function",
			Function: openaiToolFunction{
				Name:        d.Name,
				Description: d.Description,
				Parameters:  d.InputSchema,
			},
		})
	}
	return out
}

type openaiMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type openaiChatResponse struct {
	Choices []struct {
		Index        int           `json:"index"`
		Message      openaiMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
}

type openaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Role      string     `json:"role,omitempty"`
			Content   string     `json:"content,omitempty"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func toOpenAIMessages(msgs []Message) []openaiMessage {
	result := make([]openaiMessage, len(msgs))
	for i, m := range msgs {
		om := openaiMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		if len(m.ToolCalls) > 0 {
			om.ToolCalls = m.ToolCalls
		}
		result[i] = om
	}
	return result
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := openaiChatRequest{
		Model:       req.Model,
		Messages:    toOpenAIMessages(req.Messages),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}
	if req.System != "" {
		body.Messages = append([]openaiMessage{{
			Role:    "system",
			Content: req.System,
		}}, body.Messages...)
	}
	if len(req.Tools) > 0 {
		body.Tools = toOpenAITools(req.Tools)
	}

	data, _ := json.Marshal(body)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", strings.NewReader(string(data)))
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(respBody))
	}

	var oaiResp openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := oaiResp.Choices[0]
	result := &ChatResponse{
		Message: Message{
			Role:    RoleAssistant,
			Content: choice.Message.Content,
		},
		Usage: Usage{
			InputTokens:  oaiResp.Usage.PromptTokens,
			OutputTokens: oaiResp.Usage.CompletionTokens,
		},
	}
	if len(choice.Message.ToolCalls) > 0 {
		result.Message.ToolCalls = choice.Message.ToolCalls
	}
	return result, nil
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	body := openaiChatRequest{
		Model:       req.Model,
		Messages:    toOpenAIMessages(req.Messages),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      true,
	}
	if req.System != "" {
		body.Messages = append([]openaiMessage{{
			Role:    "system",
			Content: req.System,
		}}, body.Messages...)
	}
	if len(req.Tools) > 0 {
		body.Tools = toOpenAITools(req.Tools)
	}

	data, _ := json.Marshal(body)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", strings.NewReader(string(data)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("api request: %w", err)
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamEvent)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := NewSSEScanner(resp.Body)
		for scanner.Scan() {
			event := scanner.Event()
			if event == nil {
				continue
			}

			if string(event.Data) == "[DONE]" {
				ch <- StreamEvent{Type: StreamDone, Done: true}
				return
			}

			var chunk openaiStreamChunk
			if err := json.Unmarshal(event.Data, &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			if delta.Content != "" {
				ch <- StreamEvent{
					Type:    StreamChunk,
					Content: delta.Content,
				}
			}

			if len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {
					tc := tc
					ch <- StreamEvent{
						Type:     StreamToolCall,
						ToolCall: &tc,
					}
				}
			}

			if chunk.Choices[0].FinishReason != nil {
				ch <- StreamEvent{Type: StreamDone, Done: true}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{Type: StreamError, Error: err}
		} else {
			ch <- StreamEvent{Type: StreamDone, Done: true}
		}
	}()

	return ch, nil
}

type AnthropicProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	return &AnthropicProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicContentBlock struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Input   any    `json:"input,omitempty"`
	Content string `json:"content,omitempty"`
}

type anthropicRequest struct {
	Model       string                 `json:"model"`
	Messages    []anthropicMessage     `json:"messages"`
	System      string                 `json:"system,omitempty"`
	MaxTokens   int                    `json:"max_tokens"`
	Temperature float64                `json:"temperature"`
	Tools       []tools.ToolDefinition `json:"tools,omitempty"`
	Stream      bool                   `json:"stream"`
}

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicStreamChunk struct {
	Type         string                 `json:"type"`
	Index        int                    `json:"index,omitempty"`
	Delta        *anthropicStreamDelta  `json:"delta,omitempty"`
	ContentBlock *anthropicContentBlock `json:"content_block,omitempty"`
}

type anthropicStreamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

func toAnthropicMessages(msgs []Message) []anthropicMessage {
	var result []anthropicMessage
	for _, m := range msgs {
		switch m.Role {
		case RoleTool:
			result = append(result, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{{
					Type:    "tool_result",
					ID:      m.ToolCallID,
					Content: m.Content,
				}},
			})
		case RoleAssistant:
			blocks := []anthropicContentBlock{}
			if m.Content != "" {
				blocks = append(blocks, anthropicContentBlock{
					Type: "text",
					Text: m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: json.RawMessage(tc.Function.Arguments),
				})
			}
			result = append(result, anthropicMessage{
				Role:    "assistant",
				Content: blocks,
			})
		case RoleUser:
			result = append(result, anthropicMessage{
				Role:    "user",
				Content: m.Content,
			})
		}
	}
	return result
}

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := anthropicRequest{
		Model:       req.Model,
		Messages:    toAnthropicMessages(req.Messages),
		System:      req.System,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      false,
	}
	if len(req.Tools) > 0 {
		body.Tools = req.Tools
	}

	data, _ := json.Marshal(body)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", strings.NewReader(string(data)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(respBody))
	}

	var anthResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	result := &ChatResponse{
		Message: Message{
			Role: RoleAssistant,
		},
		Usage: Usage{
			InputTokens:  anthResp.Usage.InputTokens,
			OutputTokens: anthResp.Usage.OutputTokens,
		},
	}

	var textParts []string
	for _, block := range anthResp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			inputJSON, _ := json.Marshal(block.Input)
			result.Message.ToolCalls = append(result.Message.ToolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: ToolCallFunction{
					Name:      block.Name,
					Arguments: string(inputJSON),
				},
			})
		}
	}
	result.Message.Content = strings.Join(textParts, "")

	return result, nil
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	body := anthropicRequest{
		Model:       req.Model,
		Messages:    toAnthropicMessages(req.Messages),
		System:      req.System,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}
	if len(req.Tools) > 0 {
		body.Tools = req.Tools
	}

	data, _ := json.Marshal(body)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", strings.NewReader(string(data)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("api request: %w", err)
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamEvent)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		// Anthropic streams tool-use arguments incrementally as
		// input_json_delta deltas. Accumulate them per content block and
		// emit one complete ToolCall per block so parallel tool calls never
		// collide and partial frames cannot corrupt the merged arguments.
		toolCalls := map[int]*ToolCall{}
		var toolOrder []int
		flushToolCalls := func() {
			for _, idx := range toolOrder {
				if tc := toolCalls[idx]; tc != nil {
					ch <- StreamEvent{Type: StreamToolCall, ToolCall: tc}
				}
			}
			toolCalls = map[int]*ToolCall{}
		}

		scanner := NewSSEScanner(resp.Body)
		for scanner.Scan() {
			event := scanner.Event()
			if event == nil {
				continue
			}

			var chunk anthropicStreamChunk
			if err := json.Unmarshal(event.Data, &chunk); err != nil {
				continue
			}

			switch chunk.Type {
			case "content_block_delta":
				if chunk.Delta == nil {
					continue
				}
				switch chunk.Delta.Type {
				case "input_json_delta":
					if chunk.Delta.PartialJSON == "" {
						continue
					}
					tc, ok := toolCalls[chunk.Index]
					if !ok {
						continue
					}
					tc.Function.Arguments += chunk.Delta.PartialJSON
				case "text_delta", "":
					if chunk.Delta.Text != "" {
						ch <- StreamEvent{
							Type:    StreamChunk,
							Content: chunk.Delta.Text,
						}
					}
				}
			case "content_block_start":
				if chunk.ContentBlock != nil && chunk.ContentBlock.Type == "tool_use" {
					tc := &ToolCall{
						ID:   chunk.ContentBlock.ID,
						Type: "function",
						Function: ToolCallFunction{
							Name: chunk.ContentBlock.Name,
						},
					}
					if _, exists := toolCalls[chunk.Index]; !exists {
						toolOrder = append(toolOrder, chunk.Index)
					}
					toolCalls[chunk.Index] = tc
				}
			case "message_stop", "message_delta":
				flushToolCalls()
				ch <- StreamEvent{Type: StreamDone, Done: true}
				return
			case "error":
				ch <- StreamEvent{Type: StreamError, Error: fmt.Errorf("anthropic error: %s", string(event.Data))}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{Type: StreamError, Error: err}
		} else {
			flushToolCalls()
			ch <- StreamEvent{Type: StreamDone, Done: true}
		}
	}()

	return ch, nil
}

type SSEMessage struct {
	Event string
	Data  []byte
}

type SSEScanner struct {
	reader   *io.PipeReader
	done     chan struct{}
	messages chan *SSEMessage
	current  *SSEMessage
	err      error
}

func NewSSEScanner(reader io.Reader) *SSEScanner {
	pr, pw := io.Pipe()
	s := &SSEScanner{
		reader:   pr,
		done:     make(chan struct{}),
		messages: make(chan *SSEMessage, 64),
	}

	go func() {
		defer pw.Close()
		io.Copy(pw, reader)
	}()

	go s.process()
	return s
}

func (s *SSEScanner) process() {
	defer close(s.messages)

	buf := make([]byte, 4096)
	var current *SSEMessage
	var leftover []byte

	for {
		n, err := s.reader.Read(buf)
		if n > 0 {
			data := append(leftover, buf[:n]...)
			lines := strings.Split(string(data), "\n")
			leftover = []byte(lines[len(lines)-1])

			for _, line := range lines[:len(lines)-1] {
				line = strings.TrimRight(line, "\r")
				if strings.HasPrefix(line, "data: ") {
					dataStr := strings.TrimPrefix(line, "data: ")
					if current == nil {
						current = &SSEMessage{}
					}
					current.Data = append(current.Data, []byte(dataStr)...)
				} else if strings.HasPrefix(line, "event: ") {
					if current == nil {
						current = &SSEMessage{}
					}
					current.Event = strings.TrimPrefix(line, "event: ")
				} else if line == "" {
					if current != nil {
						s.messages <- current
						current = nil
					}
				}
			}
		}
		if err != nil {
			if current != nil {
				s.messages <- current
			}
			s.err = err
			return
		}
	}
}

func (s *SSEScanner) Scan() bool {
	msg, ok := <-s.messages
	if !ok {
		return false
	}
	s.current = msg
	return msg != nil
}

func (s *SSEScanner) Event() *SSEMessage {
	return s.current
}

func (s *SSEScanner) Err() error {
	if s.err == io.EOF {
		return nil
	}
	return s.err
}
