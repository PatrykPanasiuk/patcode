package permissions

import (
	"fmt"
	"strings"
)

var testPrefixes = []string{
	"go test",
	"go vet",
	"go build",
	"go tool",
	"npm test",
	"npm run test",
	"npm run lint",
	"pnpm test",
	"pnpm lint",
	"yarn test",
	"yarn lint",
	"composer test",
	"vendor/bin/phpunit",
	"phpunit",
	"pytest",
	"python -m pytest",
	"make test",
	"make lint",
	"cargo test",
	"cargo check",
}

var limitedPrefixes = append([]string{
	"git status",
	"git diff",
	"git log",
	"git show",
	"git branch",
	"ls",
	"pwd",
	"find",
	"rg",
	"cat ",
	"head ",
	"tail ",
	"wc ",
	"echo ",
	"which ",
}, testPrefixes...)

var globalDenylist = []string{
	"rm ", "sudo ", "curl ", "wget ", "ssh ", "scp ", "nc ",
	"chmod ", "chown ", "dd ", "mkfs ", "mount ", "umount ",
	"kubectl ", "aws ", "gcloud ", "az ",
}

func containsDeniedPrefix(cmd string, denylist []string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	for _, d := range denylist {
		if strings.HasPrefix(lower, d) {
			return d, true
		}
	}
	return "", false
}

func CheckBashCommand(cmd string, policy BashPolicyKind) error {
	if policy == BashDeny || policy == BashAsk {
		return fmt.Errorf("bash execution is not allowed in this mode (policy: %s)", policy)
	}
	if policy == BashDirect {
		return nil
	}

	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return fmt.Errorf("empty command")
	}

	if prefix, denied := containsDeniedPrefix(trimmed, globalDenylist); denied {
		return fmt.Errorf("command %q is denied (matched prefix %q)", trimmed, prefix)
	}

	switch policy {
	case BashTestOnly:
		if _, ok := containsDeniedPrefix(trimmed, testPrefixes); ok {
			return nil
		}
		return fmt.Errorf(
			"command %q is not allowed in test-only mode; allowed: go test, npm test, pytest, etc.",
			trimmed,
		)
	case BashLimited:
		if _, ok := containsDeniedPrefix(trimmed, limitedPrefixes); ok {
			return nil
		}
		if strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "/") {
			return nil
		}
		return fmt.Errorf(
			"command %q is not allowed in limited bash mode; allowed: git, ls, cat, test commands, etc.",
			trimmed,
		)
	default:
		return fmt.Errorf("unknown bash policy: %s", policy)
	}
}
