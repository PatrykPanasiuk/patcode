package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type EditArgs struct {
	FilePath  string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func EditTool(workdir string) Tool {
	return Tool{
		Name:        "edit",
		Description: "Perform an exact string replacement in a file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to edit",
				},
				"old_string": map[string]any{
					"type":        "string",
					"description": "The exact text to search for and replace",
				},
				"new_string": map[string]any{
					"type":        "string",
					"description": "The replacement text",
				},
			},
			"required": []string{"file_path", "old_string", "new_string"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var editArgs EditArgs
			if err := json.Unmarshal(args, &editArgs); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("invalid arguments: %v", err),
				}
			}

			filePath, err := confinePath(workdir, editArgs.FilePath)
			if err != nil {
				return &ToolResult{Success: false, Error: err.Error()}
			}

			data, err := os.ReadFile(filePath)
			if err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("reading file: %v", err),
				}
			}

			content := string(data)
			if !strings.Contains(content, editArgs.OldString) {
				return &ToolResult{
					Success: false,
					Error:   "old_string not found in file content",
				}
			}

			newContent := strings.Replace(content, editArgs.OldString, editArgs.NewString, 1)
			if newContent == content {
				return &ToolResult{
					Success: false,
					Error:   "nothing was replaced (old_string matches but replacement is identical)",
				}
			}

			if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("writing file: %v", err),
				}
			}

			return &ToolResult{
				Success: true,
				Data:    "Successfully applied edit to file",
			}
		},
	}
}
