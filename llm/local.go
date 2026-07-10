package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type LocalProvider struct {
	modelPath string
	ready     bool
}

func NewLocalProvider(modelPath string, opts ...LocalOption) (*LocalProvider, error) {
	if modelPath == "" {
		return nil, fmt.Errorf("model path required (path to .gguf file)")
	}
	p := &LocalProvider{modelPath: modelPath}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

type LocalOption func(*LocalProvider)

func WithContextSize(n int) LocalOption     { return func(p *LocalProvider) {} }
func WithThreads(n int) LocalOption         { return func(p *LocalProvider) {} }
func WithGPULayers(n int) LocalOption       { return func(p *LocalProvider) {} }

func (p *LocalProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("use ChatStream")
}

func (p *LocalProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 32)

	go func() {
		defer close(ch)

		if !p.ready {
			ch <- StreamEvent{
				Type:    StreamChunk,
				Content: "[Local provider: no GGUF model loaded. To enable: install Ollama and use provider: ollama, or implement llama.cpp CGO bindings.]\n\n",
			}
			ch <- StreamEvent{Type: StreamDone, Done: true}
			return
		}

		prompt := p.buildPrompt(req.Messages, req.System)
		words := strings.Fields(prompt)
		for _, w := range words {
			select {
			case <-ctx.Done():
				ch <- StreamEvent{Type: StreamDone, Done: true}
				return
			default:
			}
			ch <- StreamEvent{Type: StreamChunk, Content: w + " "}
			time.Sleep(30 * time.Millisecond)
		}
		ch <- StreamEvent{Type: StreamDone, Done: true}
	}()

	return ch, nil
}

func (p *LocalProvider) buildPrompt(msgs []Message, system string) string {
	var prompt string
	if system != "" {
		prompt += "<|system|>\n" + system + "\n"
	}
	for _, m := range msgs {
		switch m.Role {
		case RoleUser:
			prompt += "<|user|>\n" + m.Content + "\n"
		case RoleAssistant:
			prompt += "<|assistant|>\n" + m.Content + "\n"
		case RoleSystem:
			prompt += "<|system|>\n" + m.Content + "\n"
		}
	}
	prompt += "<|assistant|>\n"
	return prompt
}

func (p *LocalProvider) Close() {}
