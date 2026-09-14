package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ServeArgs configures a long-running local process that patcode keeps alive
// across turns (like opencode's serve/local-server capability). It is
// argv-based like BashTool: no shell metacharacters are ever required, because
// patcode manages the process lifetime itself instead of relying on &/nohup.
type ServeArgs struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Port    int      `json:"port"`
	Ready   *string  `json:"ready,omitempty"`
	Workdir string   `json:"workdir,omitempty"`
	Timeout int      `json:"timeout,omitempty"`
}

type StopServeArgs struct {
	Port int `json:"port"`
}

// lockedBuf is a mutex-guarded buffer; it is safe to hand to cmd.Stdout
// (written by the child process goroutine) while readers call String().
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type ServeProcess struct {
	Cmd  *exec.Cmd
	Port int
	Logs *lockedBuf
	mu   sync.Mutex
}

var (
	serveMu    sync.Mutex
	serveProcs = map[int]*ServeProcess{}
)

// ServeTool starts the given argv command in the background (workdir-scoped,
// argv-based, no shell) and polls `ready` until the TCP port accepts
// connections. Returns the URL and PID once the service is up.
func ServeTool(workdir string) Tool {
	return Tool{
		Name:        "serve",
		Description: "Start a long-running local HTTP/API server (e.g. php -S 127.0.0.1:8080 -t public, python3 -m http.server 8080) and wait until its port is ready. Returns the base URL and PID; use stop_serve to terminate. argv-based — no shell metacharacters needed.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The command to run, e.g. \"php -S 127.0.0.1:8080 -t public\" or \"python3 -m http.server 8080\". Quoting supported; no pipes/redirects/&/;/$(...) — not needed, patcode manages the background process.",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Explicit argv (no parsing). Overrides `command` shell-word parsing.",
				},
				"port": map[string]any{
					"type":        "integer",
					"description": "TCP port to health-check until it accepts connections.",
				},
				"ready": map[string]any{
					"type":        "string",
					"description": "Optional path to poll over HTTP once the port is open, e.g. \"/health\". If set, serve polls GET http://127.0.0.1:PORT<path> until a 2xx.",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Readiness timeout in milliseconds (default: 30000).",
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": "Working directory for the server (default: project root).",
				},
			},
			"required": []string{"command", "port"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var s ServeArgs
			if err := json.Unmarshal(args, &s); err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("invalid arguments: %v", err)}
			}
			if s.Port <= 0 || s.Command == "" {
				return &ToolResult{Success: false, Error: "command and a valid port are required"}
			}

			var argv []string
			if len(s.Args) > 0 {
				argv = s.Args
			} else {
				var err error
				argv, err = shellWords(s.Command)
				if err != nil {
					return &ToolResult{Success: false, Error: err.Error()}
				}
				if hasUnsafeShellChars(s.Command) {
					return &ToolResult{
						Success: false,
						Error: "command contains shell metacharacters; serve is argv-based without a shell. " +
							"patcode manages the background process itself — drop `&`, pipes, redirects.",
					}
				}
			}
			if len(argv) == 0 {
				return &ToolResult{Success: false, Error: "empty command"}
			}

			cmdWorkdir := workdir
			if s.Workdir != "" {
				cmdWorkdir = s.Workdir
			}

			logs := &lockedBuf{}
			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
			cmd.Dir = cmdWorkdir
			cmd.Stdout = logs
			cmd.Stderr = logs

			if err := cmd.Start(); err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("failed to start: %v", err)}
			}

			serveMu.Lock()
			serveProcs[s.Port] = &ServeProcess{Cmd: cmd, Port: s.Port, Logs: logs}
			serveMu.Unlock()

			timeout := 30000
			if s.Timeout > 0 {
				timeout = s.Timeout
			}
			deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)

			for time.Now().Before(deadline) {
				select {
				case <-ctx.Done():
					ServeToolStop(ctx, s.Port)
					return &ToolResult{Success: false, Error: "serve cancelled"}
				default:
				}
				ready := ""
				if s.Ready != nil {
					ready = *s.Ready
				}
				if ready != "" {
					if httpReady(ctx, s.Port, ready) {
						return &ToolResult{
							Success: true,
							Data: fmt.Sprintf(
								"Serving at http://127.0.0.1:%d%s (pid %d). Logs:\n%s",
								s.Port, ready, cmd.Process.Pid, logs.String(),
							),
						}
					}
				} else if portReady(ctx, s.Port) {
					return &ToolResult{
						Success: true,
						Data: fmt.Sprintf(
							"Serving at http://127.0.0.1:%d (pid %d). Logs:\n%s",
							s.Port, cmd.Process.Pid, logs.String(),
						),
					}
				}
				time.Sleep(300 * time.Millisecond)
			}

			ServeToolStop(ctx, s.Port)
			return &ToolResult{
				Success: false,
				Error: fmt.Sprintf(
					"server %q did not become ready on port %d within %dms.\nLogs:\n%s",
					argv[0], s.Port, timeout, logs.String(),
				),
			}
		},
	}
}

// StopServeTool terminates a server previously started via ServeTool, by port.
func StopServeTool(workdir string) Tool {
	return Tool{
		Name:        "stop_serve",
		Description: "Stop a local server previously started via serve, identified by port. Returns the captured log tail.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"port": map[string]any{
					"type":        "integer",
					"description": "Port of the server to stop.",
				},
			},
			"required": []string{"port"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var st StopServeArgs
			if err := json.Unmarshal(args, &st); err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("invalid arguments: %v", err)}
			}
			serveMu.Lock()
			proc, ok := serveProcs[st.Port]
			if ok {
				delete(serveProcs, st.Port)
			}
			serveMu.Unlock()
			if !ok || proc == nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("no server running on port %d", st.Port)}
			}
			proc.mu.Lock()
			if proc.Cmd.Process != nil {
				_ = proc.Cmd.Process.Kill()
				_, _ = proc.Cmd.Process.Wait()
			}
			tail := proc.Logs.String()
			proc.mu.Unlock()
			return &ToolResult{
				Success: true,
				Data:    fmt.Sprintf("Stopped server on port %d.\nLog tail:\n%s", st.Port, tail),
			}
		},
	}
}

func ServeToolStop(ctx context.Context, port int) {
	serveMu.Lock()
	proc, ok := serveProcs[port]
	if ok {
		delete(serveProcs, port)
	}
	serveMu.Unlock()
	if !ok || proc == nil || proc.Cmd.Process == nil {
		return
	}
	_ = proc.Cmd.Process.Kill()
	_, _ = proc.Cmd.Process.Wait()
}

func portReady(ctx context.Context, port int) bool {
	d := net.Dialer{Timeout: 300 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func httpReady(ctx context.Context, port int, path string) bool {
	client := &http.Client{Timeout: 600 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// --- helpers shared with other argv tools ---

func itoa(n int) string { return strconv.Itoa(n) }

func trimSpace(s string) string { return strings.TrimSpace(s) }

var _ = os.Getenv
