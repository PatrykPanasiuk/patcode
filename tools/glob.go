package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type GlobArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func GlobTool(workdir string) Tool {
	return Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The glob pattern to match (e.g. '**/*.go')",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Directory to search in (relative to workdir)",
				},
			},
			"required": []string{"pattern"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var globArgs GlobArgs
			if err := json.Unmarshal(args, &globArgs); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("invalid arguments: %v", err),
				}
			}

			searchPath := workdir
			if globArgs.Path != "" {
				if filepath.IsAbs(globArgs.Path) {
					searchPath = globArgs.Path
				} else {
					searchPath = filepath.Join(workdir, globArgs.Path)
				}
			}

			cmd := exec.CommandContext(ctx, "find", searchPath, "-type", "f")
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &bytes.Buffer{}

			if err := cmd.Run(); err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("find command failed: %v", err),
				}
			}

			pattern := globArgs.Pattern
			allFiles := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")

			var matches []string
			for _, f := range allFiles {
				f = strings.TrimSpace(f)
				if f == "" {
					continue
				}
				matched, err := filepath.Match(pattern, filepath.Base(f))
				if err != nil {
					continue
				}
				if matched {
					rel, _ := filepath.Rel(workdir, f)
					matches = append(matches, rel)
					continue
				}

				matched, err = pathMatch(pattern, f, workdir)
				if err == nil && matched {
					rel, _ := filepath.Rel(workdir, f)
					matches = append(matches, rel)
				}
			}

			sort.Strings(matches)

			result := &ToolResult{
				Success: true,
				Data:    strings.Join(matches, "\n"),
			}
			result.JSON, _ = json.Marshal(map[string]any{
				"count": len(matches),
			})

			return result
		},
	}
}

func pathMatch(pattern, filePath, baseDir string) (bool, error) {
	rel, err := filepath.Rel(baseDir, filePath)
	if err != nil {
		return false, err
	}

	parts := strings.Split(rel, string(filepath.Separator))
	patternParts := strings.Split(pattern, "/")

	return matchGlobParts(parts, patternParts), nil
}

func matchGlobParts(parts, patterns []string) bool {
	if len(patterns) == 0 {
		return len(parts) == 0
	}

	if patterns[0] == "**" {
		if len(patterns) == 1 {
			return true
		}
		for i := 0; i <= len(parts); i++ {
			if matchGlobParts(parts[i:], patterns[1:]) {
				return true
			}
		}
		return false
	}

	if len(parts) == 0 {
		return false
	}

	matched, err := filepath.Match(patterns[0], parts[0])
	if err != nil || !matched {
		return false
	}

	return matchGlobParts(parts[1:], patterns[1:])
}
