package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBashToolRejectsShellMeta(t *testing.T) {
	tool := BashTool(t.TempDir())
	args, _ := json.Marshal(map[string]string{"command": "echo a && echo b"})
	result := tool.Execute(context.Background(), args)
	if result.Success {
		t.Fatal("expected failure for shell metacharacters")
	}
	if !strings.Contains(result.Error, "metacharacters") {
		t.Errorf("expected metacharacter error, got: %s", result.Error)
	}
}

func TestBashToolExecutesArgv(t *testing.T) {
	tool := BashTool(t.TempDir())
	args, _ := json.Marshal(map[string]string{"command": "echo hello world"})
	result := tool.Execute(context.Background(), args)
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if strings.TrimSpace(result.Data) != "hello world" {
		t.Errorf("expected 'hello world', got: %q", result.Data)
	}
}

func TestBashToolHonorsWorkdir(t *testing.T) {
	dir := t.TempDir()
	tool := BashTool(dir)
	args, _ := json.Marshal(map[string]string{"command": "pwd"})
	result := tool.Execute(context.Background(), args)
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if strings.TrimSpace(result.Data) != dir {
		t.Errorf("expected pwd %q, got: %q", dir, result.Data)
	}
}

func TestWriteToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	args, _ := json.Marshal(map[string]string{
		"file_path": "../escape-me.sh",
		"content":   "#!/bin/sh\n",
	})
	result := WriteTool(root).Execute(context.Background(), args)
	if result.Success {
		t.Fatal("expected failure for write outside root")
	}
	if !strings.Contains(result.Error, "outside the project root") {
		t.Errorf("expected confinement error, got: %s", result.Error)
	}
}
