package permissions

import (
	"fmt"
	"strings"
)

type ToolPermission string

const (
	PermissionAuto    ToolPermission = "auto"
	PermissionAsk     ToolPermission = "ask"
	PermissionDeny    ToolPermission = "deny"
	PermissionLimited ToolPermission = "limited"
)

type BashPolicyKind string

const (
	BashDeny     BashPolicyKind = "deny"
	BashAsk      BashPolicyKind = "ask"
	BashLimited  BashPolicyKind = "limited"
	BashTestOnly BashPolicyKind = "test-only"
	BashDirect   BashPolicyKind = "direct"
)

type ModePolicy struct {
	Mode        string
	Description string

	Read      ToolPermission
	Grep      ToolPermission
	Glob      ToolPermission
	Write     ToolPermission
	Edit      ToolPermission
	Bash      ToolPermission
	Serve     ToolPermission
	StopServe ToolPermission
	WebFetch  ToolPermission
	WebSearch ToolPermission

	BashPolicy     BashPolicyKind
	CanModifyFiles bool
	IsReadOnly     bool
	IsHeadlessSafe bool
}

var modePolicies = map[string]ModePolicy{
	"ask": {
		Mode: "ask", Description: "General answers with optional web research",
		Read: PermissionDeny, Grep: PermissionDeny, Glob: PermissionDeny,
		Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionDeny,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashDeny, CanModifyFiles: false, IsReadOnly: true, IsHeadlessSafe: true, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"inspect": {
		Mode: "inspect", Description: "Read-only repository inspection",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionDeny,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashDeny, CanModifyFiles: false, IsReadOnly: true, IsHeadlessSafe: true, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"plan": {
		Mode: "plan", Description: "Implementation planning",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionDeny,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashDeny, CanModifyFiles: false, IsReadOnly: true, IsHeadlessSafe: true, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"review": {
		Mode: "review", Description: "Code/diff review",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionLimited,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashLimited, CanModifyFiles: false, IsReadOnly: true, IsHeadlessSafe: true, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"audit": {
		Mode: "audit", Description: "Security-focused review",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionLimited,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashLimited, CanModifyFiles: false, IsReadOnly: true, IsHeadlessSafe: true, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"patch": {
		Mode: "patch", Description: "Generate patch/diff without applying",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionDeny,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashDeny, CanModifyFiles: false, IsReadOnly: true, IsHeadlessSafe: true, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"build": {
		Mode: "build", Description: "Implement changes with approval gates",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionAsk, Edit: PermissionAsk, Bash: PermissionAsk,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashAsk, CanModifyFiles: true, IsReadOnly: false, IsHeadlessSafe: false, Serve: PermissionLimited, StopServe: PermissionLimited,
	},
	"fix": {
		Mode: "fix", Description: "Fix failing tests/errors",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionAsk, Edit: PermissionAsk, Bash: PermissionLimited,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashTestOnly, CanModifyFiles: true, IsReadOnly: false, IsHeadlessSafe: false, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"refactor": {
		Mode: "refactor", Description: "Behavior-preserving changes",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionAsk, Edit: PermissionAsk, Bash: PermissionLimited,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashTestOnly, CanModifyFiles: true, IsReadOnly: false, IsHeadlessSafe: false, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"scaffold": {
		Mode: "scaffold", Description: "Create initial structure",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionAsk, Edit: PermissionAsk, Bash: PermissionDeny,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashDeny, CanModifyFiles: true, IsReadOnly: false, IsHeadlessSafe: false, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"test": {
		Mode: "test", Description: "Run and explain tests/lints",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionLimited,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashTestOnly, CanModifyFiles: false, IsReadOnly: true, IsHeadlessSafe: true, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"ci": {
		Mode: "ci", Description: "Automation-friendly review/checks",
		Read: PermissionAuto, Grep: PermissionAuto, Glob: PermissionAuto,
		Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionLimited,
		WebFetch: PermissionAuto, WebSearch: PermissionAuto,
		BashPolicy: BashTestOnly, CanModifyFiles: false, IsReadOnly: true, IsHeadlessSafe: true, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
	"shell": {
		Mode: "shell", Description: "Direct shell passthrough, no LLM",
		Read: PermissionDeny, Grep: PermissionDeny, Glob: PermissionDeny,
		Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionDeny,
		WebFetch: PermissionDeny, WebSearch: PermissionDeny,
		BashPolicy: BashDirect, CanModifyFiles: false, IsReadOnly: false, IsHeadlessSafe: false, Serve: PermissionDeny, StopServe: PermissionDeny,
	},
}

var lockedPolicy = ModePolicy{
	Mode: "unknown", Description: "Unknown mode — all tools denied",
	Read: PermissionDeny, Grep: PermissionDeny, Glob: PermissionDeny,
	Write: PermissionDeny, Edit: PermissionDeny, Bash: PermissionDeny,
	WebFetch: PermissionDeny, WebSearch: PermissionDeny,
	BashPolicy: BashDeny, CanModifyFiles: false, IsReadOnly: true, IsHeadlessSafe: true, Serve: PermissionDeny, StopServe: PermissionDeny,
}

func PolicyForMode(mode string) ModePolicy {
	if p, ok := modePolicies[mode]; ok {
		return p
	}
	locked := lockedPolicy
	locked.Mode = mode
	return locked
}

func ToolsAllowedForMode(mode string) map[string]ToolPermission {
	p := PolicyForMode(mode)
	return map[string]ToolPermission{
		"read": p.Read, "grep": p.Grep, "glob": p.Glob,
		"write": p.Write, "edit": p.Edit, "bash": p.Bash,
		"webfetch": p.WebFetch, "websearch": p.WebSearch, "serve": p.Serve, "stop_serve": p.StopServe,
	}
}

func IsToolAllowed(mode string, toolName string) ToolPermission {
	toolName = strings.ToLower(toolName)
	m := ToolsAllowedForMode(mode)
	if p, ok := m[toolName]; ok {
		return p
	}
	return PermissionDeny
}

func BashPolicyForMode(mode string) BashPolicyKind {
	return PolicyForMode(mode).BashPolicy
}

func AllowedToolNames(mode string) []string {
	p := PolicyForMode(mode)
	var names []string
	for name, perm := range map[string]ToolPermission{
		"read": p.Read, "grep": p.Grep, "glob": p.Glob,
		"write": p.Write, "edit": p.Edit, "bash": p.Bash,
		"webfetch": p.WebFetch, "websearch": p.WebSearch, "serve": p.Serve, "stop_serve": p.StopServe,
	} {
		if perm != PermissionDeny {
			names = append(names, name)
		}
	}
	return names
}

func ToolDeniedError(mode string, toolName string) string {
	return fmt.Sprintf("tool %q is not allowed in mode %q", toolName, mode)
}

func ToolAskError(mode string, toolName string) string {
	return fmt.Sprintf(
		"tool %q requires approval in mode %q; approval flow is not implemented for headless mode yet",
		toolName, mode,
	)
}
