package session

import (
	"strings"
	"testing"
)

func TestParseMode_AllModes(t *testing.T) {
	for _, m := range allModes {
		parsed, err := ParseMode(string(m))
		if err != nil {
			t.Errorf("ParseMode(%q) unexpected error: %v", m, err)
		}
		if parsed != m {
			t.Errorf("ParseMode(%q) = %q, want %q", m, parsed, m)
		}
	}
}

func TestParseMode_CaseSensitive(t *testing.T) {
	_, err := ParseMode("ASK")
	if err == nil {
		t.Error("expected error for uppercase ASK")
	}
}

func TestParseMode_Unknown(t *testing.T) {
	_, err := ParseMode("foobar")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown mode") {
		t.Errorf("expected 'unknown mode' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ask") {
		t.Errorf("expected available modes listed in error, got: %v", err)
	}
}

func TestAllModes_Count(t *testing.T) {
	modes := AllModes()
	if len(modes) != 13 {
		t.Errorf("expected 13 modes, got %d", len(modes))
	}
}

func TestAllModes_Contains(t *testing.T) {
	modes := AllModes()
	modeSet := make(map[Mode]bool)
	for _, m := range modes {
		modeSet[m] = true
	}
	required := []Mode{ModeAsk, ModeInspect, ModePlan, ModeReview, ModeAudit,
		ModePatch, ModeBuild, ModeFix, ModeRefactor, ModeScaffold, ModeTest, ModeCI, ModeShell}
	for _, r := range required {
		if !modeSet[r] {
			t.Errorf("missing mode %q", r)
		}
	}
}

func TestMode_String(t *testing.T) {
	if ModeAsk.String() != "ask" {
		t.Errorf("expected 'ask', got %q", ModeAsk.String())
	}
	if ModeBuild.String() != "build" {
		t.Errorf("expected 'build', got %q", ModeBuild.String())
	}
}

func TestParseMode_BackwardCompat(t *testing.T) {
	// Old modes still work
	for _, m := range []Mode{ModeAsk, ModePlan, ModeBuild, ModeShell} {
		parsed, err := ParseMode(string(m))
		if err != nil {
			t.Errorf("ParseMode(%q) backward compat error: %v", m, err)
		}
		if parsed != m {
			t.Errorf("ParseMode(%q) = %q", m, parsed)
		}
	}
}
