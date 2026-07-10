package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type BashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
	Workdir string `json:"workdir,omitempty"`
}

func BashTool(workdir string) Tool {
	return Tool{
		Name:        "bash",
		Description: "Execute a shell command with optional timeout and working directory",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The shell command to execute",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in milliseconds (default: 120000)",
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": "Working directory for the command",
				},
			},
			"required": []string{"command"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var bashArgs BashArgs
			if err := json.Unmarshal(args, &bashArgs); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("invalid arguments: %v", err),
				}
			}

			if bashArgs.Command == "" {
				return &ToolResult{
					Success: false,
					Error:   "command is required",
				}
			}

			timeout := 120000
			if bashArgs.Timeout > 0 {
				timeout = bashArgs.Timeout
			}

			cmdWorkdir := workdir
			if bashArgs.Workdir != "" {
				cmdWorkdir = bashArgs.Workdir
			}

			ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", "-c", bashArgs.Command)
			cmd.Dir = cmdWorkdir

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			result := &ToolResult{
				Success: err == nil,
			}

			var output string
			if stdout.Len() > 0 {
				output = stdout.String()
			}
			if stderr.Len() > 0 {
				if output != "" {
					output += "\n"
				}
				output += "STDERR:\n" + stderr.String()
			}

			if ctx.Err() == context.DeadlineExceeded {
				result.Error = fmt.Sprintf("command timed out after %dms", timeout)
				result.Data = output
				return result
			}

			if err != nil {
				result.Error = fmt.Sprintf("command failed: %v", err)
				result.Data = output
				return result
			}

			result.Data = output
			result.JSON, _ = json.Marshal(map[string]any{
				"exit_code": 0,
				"stdout":    stdout.String(),
				"stderr":    stderr.String(),
			})

			return result
		},
	}
}
