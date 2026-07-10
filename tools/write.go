package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WriteArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func WriteTool(workdir string) Tool {
	return Tool{
		Name:        "write",
		Description: "Create or overwrite a file with new content",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to write",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The content to write to the file",
				},
			},
			"required": []string{"file_path", "content"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var writeArgs WriteArgs
			if err := json.Unmarshal(args, &writeArgs); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("invalid arguments: %v", err),
				}
			}

			filePath := writeArgs.FilePath
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(workdir, filePath)
			}

			dir := filepath.Dir(filePath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("creating directory: %v", err),
				}
			}

			if err := os.WriteFile(filePath, []byte(writeArgs.Content), 0644); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("writing file: %v", err),
				}
			}

			return &ToolResult{
				Success: true,
				Data:    fmt.Sprintf("Successfully wrote %d bytes to %s", len(writeArgs.Content), filePath),
			}
		},
	}
}
