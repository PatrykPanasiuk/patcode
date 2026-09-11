package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// confinePath resolves target against root and guarantees that the path the
// tool will operate on stays inside the project root. Any symlink that would
// resolve outside root is rejected. For paths that do not exist yet (e.g.
// files being written), the deepest existing ancestor is resolved and checked
// instead, so a symlinked parent cannot smuggle a write out of the project.
func confinePath(root, target string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("empty path")
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolving root: %w", err)
	}
	rootAbs = filepath.Clean(rootAbs)

	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(rootAbs, abs)
	}
	abs = filepath.Clean(abs)

	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		rootReal = rootAbs
	}

	var missing []string
	cur := abs
	for {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			full := real
			for i := len(missing) - 1; i >= 0; i-- {
				full = filepath.Join(full, missing[i])
			}
			if !pathWithinRoot(full, rootReal) {
				return "", fmt.Errorf(
					"path %q resolves outside the project root %q", target, rootAbs,
				)
			}
			return abs, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolving path %q: %w", abs, err)
		}

		base := filepath.Base(cur)
		if base == "." || base == string(filepath.Separator) || base == "" {
			return "", fmt.Errorf(
				"cannot confine %q within project root %q", target, rootAbs,
			)
		}
		missing = append([]string{base}, missing...)
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf(
				"cannot confine %q within project root %q", target, rootAbs,
			)
		}
		cur = parent
	}
}

func pathWithinRoot(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	rel = filepath.Clean(rel)
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
