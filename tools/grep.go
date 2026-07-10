package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type GrepArgs struct {
	Pattern string `json:"pattern"`
	Include string `json:"include,omitempty"`
	Path    string `json:"path,omitempty"`
}

func GrepTool(workdir string) Tool {
	return Tool{
		Name:        "grep",
		Description: "Search file contents using regex patterns",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The regex pattern to search for",
				},
				"include": map[string]any{
					"type":        "string",
					"description": "File pattern to filter (e.g. '*.go', '*.{ts,tsx}')",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Directory to search in (relative to workdir)",
				},
			},
			"required": []string{"pattern"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var grepArgs GrepArgs
			if err := json.Unmarshal(args, &grepArgs); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("invalid arguments: %v", err),
				}
			}

			searchPath := workdir
			if grepArgs.Path != "" {
				if filepath.IsAbs(grepArgs.Path) {
					searchPath = grepArgs.Path
				} else {
					searchPath = filepath.Join(workdir, grepArgs.Path)
				}
			}

			rgArgs := []string{"--no-heading", "-n"}
			if grepArgs.Include != "" {
				rgArgs = append(rgArgs, "-g", grepArgs.Include)
			}
			rgArgs = append(rgArgs, grepArgs.Pattern, searchPath)

			cmd := exec.CommandContext(ctx, "rg", rgArgs...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			result := &ToolResult{Success: true}

			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if exitErr.ExitCode() == 1 && stderr.Len() == 0 {
						result.Data = "No matches found."
						result.JSON, _ = json.Marshal(map[string]any{
							"count": 0,
						})
						return result
					}
				}
				if stderr.Len() > 0 {
					result.Data = stderr.String()
				} else {
					result.Success = false
					result.Error = fmt.Sprintf("grep failed: %v", err)
					return result
				}
			}

			output := strings.TrimRight(stdout.String(), "\n")
			if output == "" {
				output = "No matches found."
			}

			lineCount := 0
			if output != "" {
				lineCount = len(strings.Split(output, "\n"))
			}

			result.Data = output
			result.JSON, _ = json.Marshal(map[string]any{
				"count": lineCount,
			})

			return result
		},
	}
}
