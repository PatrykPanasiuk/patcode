package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveProjectDir finds the closest directory that contains patcode.yaml.
// It first checks the provided path, then walks up parents, and finally tries
// a common local fallback at <cwd>/patcode for this workspace layout.
func ResolveProjectDir(start string) (string, error) {
	if start == "" {
		start = "."
	}

	abs, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolving project path: %w", err)
	}

	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	if isProjectDir(abs) {
		return abs, nil
	}

	for dir := abs; ; dir = filepath.Dir(dir) {
		if isProjectDir(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	fallback := filepath.Join(abs, "patcode")
	if isProjectDir(fallback) {
		return fallback, nil
	}

	return "", fmt.Errorf("could not find patcode.yaml starting from %s", abs)
}

func isProjectDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "patcode.yaml"))
	return err == nil && !info.IsDir()
}
