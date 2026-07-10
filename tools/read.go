package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ReadArgs struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func ReadTool(workdir string) Tool {
	return Tool{
		Name:        "read",
		Description: "Read the contents of a file with optional offset and limit",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to read",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Line number to start reading from (1-indexed)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of lines to read",
				},
			},
			"required": []string{"file_path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var readArgs ReadArgs
			if err := json.Unmarshal(args, &readArgs); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("invalid arguments: %v", err),
				}
			}

			filePath := readArgs.FilePath
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(workdir, filePath)
			}

			data, err := os.ReadFile(filePath)
			if err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("reading file: %v", err),
				}
			}

			lines := bytes.Split(data, []byte("\n"))
			startLine := readArgs.Offset
			if startLine < 1 {
				startLine = 1
			}

			endLine := len(lines)
			if readArgs.Limit > 0 && startLine+readArgs.Limit-1 < endLine {
				endLine = startLine + readArgs.Limit - 1
			}

			if startLine > len(lines) {
				return &ToolResult{
					Success: true,
					Data:    fmt.Sprintf("File has %d lines, requested start at %d", len(lines), startLine),
				}
			}

			var output bytes.Buffer
			for i := startLine - 1; i < endLine; i++ {
				output.WriteString(fmt.Sprintf("%d: %s\n", i+1, string(lines[i])))
			}

			result := &ToolResult{
				Success: true,
				Data:    output.String(),
			}
			result.JSON, _ = json.Marshal(map[string]any{
				"total_lines": len(lines),
				"start_line":  startLine,
				"end_line":    endLine,
			})

			return result
		},
	}
}
