package tools

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// freePortT returns a currently-unused TCP port on 127.0.0.1.
func freePortT(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// TestServeLocalPythonHTTPServer proves patcode can start a long-running
// local HTTP endpoint, wait until it's ready on its port, and tear it down —
// the exact capability opencode has natively (serve/dev-server), previously
// impossible for patcode because its bash tool is argv-based, rejects `&`, and
// relies on a 120s foreground timeout. serve + stop_serve close that gap
// while staying argv-based (no shell metacharacters ever required).
func TestServeLocalPythonHTTPServer(t *testing.T) {
	port := freePortT(t)
	workdir := t.TempDir()

	srv := ServeTool(workdir)
	args, _ := json.Marshal(map[string]any{
		"command": "python3 -m http.server " + strconv.Itoa(port) + " --bind 127.0.0.1",
		"port":    port,
		"timeout": 30000,
	})
	res := srv.Execute(context.Background(), args)
	if !res.Success {
		t.Fatalf("serve failed: %s (%s)", res.Error, res.Data)
	}
	var ok bool
	for attempt := 0; attempt < 40; attempt++ {
		resp := httpGET(port)
		if resp != nil {
			ok = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("HTTP GET on http://127.0.0.1:%d failed after retries", port)
	}

	// Health-check endpoint on the port.
	for retry := 0; ; retry++ {
		resp := httpGET(port)
		if resp != nil {
			break
		}
		if retry >= 60 {
			t.Fatalf("HTTP GET on http://127.0.0.1:%d failed after retries", port)
		}
		time.Sleep(250 * time.Millisecond)
	}

	stop := StopServeTool(workdir)
	stopArgs, _ := json.Marshal(map[string]any{"port": port})
	sr := stop.Execute(context.Background(), stopArgs)
	if !sr.Success {
		t.Fatalf("stop_serve failed: %s (%s)", sr.Error, sr.Data)
	}

	// Port must be free again shortly after stop.
	deadline := time.Now().Add(5 * time.Second)
	for !portClosed(port) {
		if time.Now().After(deadline) {
			t.Fatalf("port %d still open after stop_serve", port)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func portClosed(port int) bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 300*time.Millisecond)
	if err != nil {
		return true
	}
	_ = conn.Close()
	return false
}

// httpGET returns a non-nil resp if HEAD/GET succeeds on the port, else nil.
func httpGET(port int) *http.Response {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp
	}
	return nil
}

// TestServeRejectsShellMetacharacters enforces patcode's argv-only model on
// the serve tool too: no `&`, `|`, `>` or `;` — patcode owns the background
// process, so shell syntax is never needed.
func TestServeRejectsShellMetacharacters(t *testing.T) {
	port := freePortT(t)
	srv := ServeTool(t.TempDir())
	for _, cmd := range []string{
		"python3 -m http.server &",
		"python3 -m http.server" + " " + strconv.Itoa(port) + " | tee /tmp/x",
		"python3 -m http.server > /tmp/log",
	} {
		args, _ := json.Marshal(map[string]any{"command": cmd, "port": port})
		res := srv.Execute(context.Background(), args)
		if res.Success {
			t.Fatalf("serve should reject %q", cmd)
		}
	}
}
