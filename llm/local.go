package llm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LocalProvider runs real GGUF inference by delegating to an external
// llama.cpp `llama-server` instance exposing the OpenAI-compatible HTTP API.
// It either attaches to a running server (PATCODE_LLAMA_SERVER_URL or
// base_url in config) or spawns one from a local llama-server binary.
type LocalProvider struct {
	mu         sync.Mutex
	openai     *OpenAIProvider
	serverURL  string
	serverProc *exec.Cmd
}

type LocalOption func(*LocalProvider)

// WithServerURL points the provider at an already-running OpenAI-compatible
// server, disabling auto-start.
func WithServerURL(url string) LocalOption {
	return func(p *LocalProvider) { p.serverURL = strings.TrimRight(url, "/") }
}

// WithLlamaBin overrides the llama-server binary path used for auto-start.
func WithLlamaBin(path string) LocalOption {
	return func(p *LocalProvider) {}
}

// WithPort selects the listen port for an auto-started llama-server.
func WithPort(port int) LocalOption {
	return func(p *LocalProvider) {}
}

func WithContextSize(n int) LocalOption { return func(p *LocalProvider) {} }
func WithThreads(n int) LocalOption     { return func(p *LocalProvider) {} }
func WithGPULayers(n int) LocalOption   { return func(p *LocalProvider) {} }

func NewLocalProvider(modelPath string, opts ...LocalOption) (*LocalProvider, error) {
	p := &LocalProvider{}
	for _, opt := range opts {
		opt(p)
	}

	serverURL := p.serverURL
	if serverURL == "" {
		serverURL = strings.TrimRight(os.Getenv("PATCODE_LLAMA_SERVER_URL"), "/")
	}

	switch {
	case serverURL != "":
		p.serverURL = serverURL
		p.openai = NewOpenAIProvider("", serverURL)
		return p, nil

	case modelPath == "":
		return nil, fmt.Errorf(
			"local provider requires either PATCODE_LLAMA_SERVER_URL (a running llama-server) or model_path pointing at a .gguf file",
		)

	default:
		bin := os.Getenv("PATCODE_LLAMA_BIN")
		if bin == "" {
			bin = "llama-server"
		}
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf(
				"cannot find llama-server binary %q: %w\n\n"+
					"To use the local GGUF provider, either:\n"+
					"  - set PATCODE_LLAMA_SERVER_URL to a running llama-server (OpenAI-compatible), or\n"+
					"  - install llama.cpp (get llama-server on your PATH): https://github.com/ggml-org/llama.cpp",
				bin, err,
			)
		}

		if err := p.startServer(bin, modelPath); err != nil {
			return nil, err
		}
		return p, nil
	}
}

func (p *LocalProvider) startServer(bin, modelPath string) error {
	port := os.Getenv("PATCODE_LLAMA_PORT")
	if port == "" {
		port = "8080"
	}
	if !validPort(port) {
		return fmt.Errorf("invalid PATCODE_LLAMA_PORT %q", port)
	}

	if !filepath.IsAbs(modelPath) {
		abs, err := filepath.Abs(modelPath)
		if err == nil {
			modelPath = abs
		}
	}

	ctxSize := os.Getenv("PATCODE_LLAMA_CTX")
	if ctxSize == "" {
		ctxSize = "4096"
	}
	threads := os.Getenv("PATCODE_LLAMA_THREADS")
	if threads == "" {
		threads = "4"
	}
	gpuLayers := os.Getenv("PATCODE_LLAMA_GPU_LAYERS")
	if gpuLayers == "" {
		gpuLayers = "0"
	}

	args := []string{
		"-m", modelPath,
		"--host", "127.0.0.1",
		"--port", port,
		"--ctx-size", ctxSize,
		"--threads", threads,
		"--n-gpu-layers", gpuLayers,
	}

	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting llama-server: %w", err)
	}
	p.serverProc = cmd

	serverURL := fmt.Sprintf("http://127.0.0.1:%s/v1", port)
	p.serverURL = serverURL
	p.openai = NewOpenAIProvider("", serverURL)

	healthURL := fmt.Sprintf("http://127.0.0.1:%s/health", port)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	p.Close()
	return fmt.Errorf(
		"llama-server did not become healthy within 120s at %s; check the model file and server logs",
		healthURL,
	)
}

func validPort(port string) bool {
	if port == "" {
		return false
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := net.LookupPort("tcp", port)
	if err != nil {
		return false
	}
	return n > 0
}

func (p *LocalProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.openai == nil {
		return nil, fmt.Errorf("local provider is not ready")
	}
	return p.openai.Chat(ctx, req)
}

func (p *LocalProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.openai == nil {
		return nil, fmt.Errorf("local provider is not ready")
	}
	return p.openai.ChatStream(ctx, req)
}

func (p *LocalProvider) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.serverProc != nil && p.serverProc.Process != nil {
		p.serverProc.Process.Kill()
		p.serverProc = nil
	}
}

// ServerURL returns the base URL the provider talks to (useful for the TUI).
func (p *LocalProvider) ServerURL() string {
	return p.serverURL
}
