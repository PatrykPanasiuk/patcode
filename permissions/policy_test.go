package permissions

import (
	"strings"
	"testing"
)

func TestPolicyForMode_AllModes(t *testing.T) {
	modes := []string{"ask", "inspect", "plan", "review", "audit", "patch",
		"build", "fix", "refactor", "scaffold", "test", "ci", "shell"}
	for _, m := range modes {
		p := PolicyForMode(m)
		if p.Mode != m {
			t.Errorf("PolicyForMode(%q).Mode = %q", m, p.Mode)
		}
		if p.Description == "" {
			t.Errorf("PolicyForMode(%q).Description is empty", m)
		}
	}
}

func TestPolicyForMode_Unknown(t *testing.T) {
	p := PolicyForMode("nonexistent")
	if p.Mode != "ask" {
		t.Errorf("expected fallback to ask, got %q", p.Mode)
	}
}

func TestIsToolAllowed_Ask(t *testing.T) {
	perm := IsToolAllowed("ask", "read")
	if perm != PermissionDeny {
		t.Errorf("expected deny, got %s", perm)
	}
	perm = IsToolAllowed("ask", "write")
	if perm != PermissionDeny {
		t.Errorf("expected deny, got %s", perm)
	}
}

func TestIsToolAllowed_Inspect(t *testing.T) {
	for _, tool := range []string{"read", "grep", "glob"} {
		perm := IsToolAllowed("inspect", tool)
		if perm != PermissionAuto {
			t.Errorf("inspect %s: expected auto, got %s", tool, perm)
		}
	}
	for _, tool := range []string{"write", "edit", "bash"} {
		perm := IsToolAllowed("inspect", tool)
		if perm != PermissionDeny {
			t.Errorf("inspect %s: expected deny, got %s", tool, perm)
		}
	}
}

func TestIsToolAllowed_Build(t *testing.T) {
	for _, tool := range []string{"read", "grep", "glob"} {
		perm := IsToolAllowed("build", tool)
		if perm != PermissionAuto {
			t.Errorf("build %s: expected auto, got %s", tool, perm)
		}
	}
	for _, tool := range []string{"write", "edit", "bash"} {
		perm := IsToolAllowed("build", tool)
		if perm != PermissionAsk {
			t.Errorf("build %s: expected ask, got %s", tool, perm)
		}
	}
}

func TestIsToolAllowed_Patch(t *testing.T) {
	for _, tool := range []string{"read", "grep", "glob"} {
		perm := IsToolAllowed("patch", tool)
		if perm != PermissionAuto {
			t.Errorf("patch %s: expected auto, got %s", tool, perm)
		}
	}
	for _, tool := range []string{"write", "edit", "bash"} {
		perm := IsToolAllowed("patch", tool)
		if perm != PermissionDeny {
			t.Errorf("patch %s: expected deny, got %s", tool, perm)
		}
	}
}

func TestIsToolAllowed_Test(t *testing.T) {
	for _, tool := range []string{"read", "grep", "glob"} {
		perm := IsToolAllowed("test", tool)
		if perm != PermissionAuto {
			t.Errorf("test %s: expected auto, got %s", tool, perm)
		}
	}
	for _, tool := range []string{"write", "edit"} {
		perm := IsToolAllowed("test", tool)
		if perm != PermissionDeny {
			t.Errorf("test %s: expected deny, got %s", tool, perm)
		}
	}
	// Bash should be limited (test-only)
	perm := IsToolAllowed("test", "bash")
	if perm != PermissionLimited {
		t.Errorf("test bash: expected limited, got %s", perm)
	}
}

func TestAllowedToolNames_Ask(t *testing.T) {
	names := AllowedToolNames("ask")
	if len(names) != 0 {
		t.Errorf("expected no tools for ask, got %v", names)
	}
}

func TestAllowedToolNames_Inspect(t *testing.T) {
	names := AllowedToolNames("inspect")
	if len(names) != 3 {
		t.Errorf("expected 3 tools for inspect, got %d: %v", len(names), names)
	}
	for _, n := range names {
		if n != "read" && n != "grep" && n != "glob" {
			t.Errorf("unexpected tool %q in inspect", n)
		}
	}
}

func TestAllowedToolNames_Build(t *testing.T) {
	names := AllowedToolNames("build")
	if len(names) != 6 {
		t.Errorf("expected 6 tools for build, got %d: %v", len(names), names)
	}
}

func TestAllowedToolNames_Patch(t *testing.T) {
	names := AllowedToolNames("patch")
	if len(names) != 3 {
		t.Errorf("expected 3 tools for patch (no write/edit/bash), got %d: %v", len(names), names)
	}
}

func TestToolDeniedError(t *testing.T) {
	err := ToolDeniedError("inspect", "write")
	if !strings.Contains(err, "write") || !strings.Contains(err, "inspect") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestToolAskError(t *testing.T) {
	err := ToolAskError("build", "edit")
	if !strings.Contains(err, "approval") {
		t.Errorf("expected approval message, got: %s", err)
	}
}
