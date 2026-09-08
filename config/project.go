package config

import (
	"os"
	"path/filepath"
)

// ResolveProjectDir resolves the absolute path for a project directory.
// Does not require patcode.yaml — config is loaded separately via LoadWithFallback.
func ResolveProjectDir(start string) (string, error) {
	if start == "" {
		start = "."
	}

	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	// If path is a file, use its parent directory
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	}

	return abs, nil
}
