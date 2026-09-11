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
	if p.Mode != "nonexistent" {
		t.Errorf("expected locked fallback policy with the original mode, got %q", p.Mode)
	}
}

func TestPolicyForMode_UnknownAllDenied(t *testing.T) {
	_ = PolicyForMode("nope")
	for _, tool := range []string{"read", "write", "edit", "bash", "grep", "glob", "webfetch", "websearch"} {
		if perm := IsToolAllowed("nope", tool); perm != PermissionDeny {
			t.Errorf("unknown mode %s: expected deny, got %s", tool, perm)
		}
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
	// Web research is available in ask mode.
	perm = IsToolAllowed("ask", "webfetch")
	if perm != PermissionAuto {
		t.Errorf("ask webfetch: expected auto, got %s", perm)
	}
	perm = IsToolAllowed("ask", "websearch")
	if perm != PermissionAuto {
		t.Errorf("ask websearch: expected auto, got %s", perm)
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
	if len(names) != 2 {
		t.Errorf("expected webfetch+websearch for ask, got %v", names)
	}
}

func TestAllowedToolNames_Inspect(t *testing.T) {
	names := AllowedToolNames("inspect")
	if len(names) != 5 {
		t.Errorf("expected 5 tools for inspect, got %d: %v", len(names), names)
	}
	for _, n := range names {
		if n != "read" && n != "grep" && n != "glob" && n != "webfetch" && n != "websearch" {
			t.Errorf("unexpected tool %q in inspect", n)
		}
	}
}

func TestAllowedToolNames_Build(t *testing.T) {
	names := AllowedToolNames("build")
	if len(names) != 8 {
		t.Errorf("expected 8 tools for build, got %d: %v", len(names), names)
	}
}

func TestAllowedToolNames_Patch(t *testing.T) {
	names := AllowedToolNames("patch")
	if len(names) != 5 {
		t.Errorf("expected 5 tools for patch (no write/edit/bash), got %d: %v", len(names), names)
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
