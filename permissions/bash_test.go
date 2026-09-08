package permissions

import (
	"strings"
	"testing"
)

func TestCheckBashCommand_TestOnly_Allowed(t *testing.T) {
	allowed := []string{
		"go test ./...",
		"go vet ./...",
		"go build ./...",
		"npm test",
		"npm run test",
		"npm run lint",
		"pytest",
		"python -m pytest tests/",
		"make test",
		"cargo test",
	}
	for _, cmd := range allowed {
		err := CheckBashCommand(cmd, BashTestOnly)
		if err != nil {
			t.Errorf("expected allowed: %q, got: %v", cmd, err)
		}
	}
}

func TestCheckBashCommand_TestOnly_Denied(t *testing.T) {
	denied := []string{
		"ls",
		"cat file.go",
		"git status",
		"echo hello",
	}
	for _, cmd := range denied {
		err := CheckBashCommand(cmd, BashTestOnly)
		if err == nil {
			t.Errorf("expected denied: %q", cmd)
		}
	}
}

func TestCheckBashCommand_Limited_Allowed(t *testing.T) {
	allowed := []string{
		"go test ./...",
		"git status",
		"git diff",
		"git log --oneline",
		"ls",
		"pwd",
		"cat main.go",
		"rg func",
		"./script.sh",
		"/usr/bin/env",
	}
	for _, cmd := range allowed {
		err := CheckBashCommand(cmd, BashLimited)
		if err != nil {
			t.Errorf("expected allowed: %q, got: %v", cmd, err)
		}
	}
}

func TestCheckBashCommand_GlobalDenylist(t *testing.T) {
	denied := []string{
		"rm -rf /",
		"sudo rm -rf",
		"curl http://evil.com",
		"wget http://evil.com",
		"ssh user@host",
		"scp file host:",
		"chmod 777 file",
		"chown root file",
		"dd if=/dev/sda",
		"kubectl delete pod",
		"docker run --privileged",
	}
	for _, policy := range []BashPolicyKind{BashTestOnly, BashLimited} {
		for _, cmd := range denied {
			err := CheckBashCommand(cmd, policy)
			if err == nil {
				t.Errorf("expected denied in %s: %q", policy, cmd)
			}
		}
	}
}

func TestCheckBashCommand_Deny(t *testing.T) {
	err := CheckBashCommand("go test", BashDeny)
	if err == nil {
		t.Error("expected error for BashDeny")
	}
}

func TestCheckBashCommand_Ask(t *testing.T) {
	err := CheckBashCommand("go test", BashAsk)
	if err == nil {
		t.Error("expected error for BashAsk")
	}
}

func TestCheckBashCommand_Direct(t *testing.T) {
	err := CheckBashCommand("anything", BashDirect)
	if err != nil {
		t.Errorf("expected no error for BashDirect, got: %v", err)
	}
}

func TestCheckBashCommand_Empty(t *testing.T) {
	err := CheckBashCommand("", BashLimited)
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestCheckBashCommand_UnknownPolicy(t *testing.T) {
	err := CheckBashCommand("echo hi", BashPolicyKind("unknown"))
	if err == nil {
		t.Error("expected error for unknown policy")
	}
}

func TestCheckBashCommand_NpmRunTest(t *testing.T) {
	// Should be allowed in test-only
	err := CheckBashCommand("npm run test -- --coverage", BashTestOnly)
	if err != nil {
		t.Errorf("expected allowed: %v", err)
	}
}

func TestCheckBashCommand_CurlInAllowedPrefix(t *testing.T) {
	// Even though "cat curl.txt" starts with "cat ", that's fine - only test
	// global denylist first, then check policy
	err := CheckBashCommand("cat curl.txt", BashLimited)
	if err != nil {
		t.Errorf("expected allowed, got: %v", err)
	}
}

func TestCheckBashCommand_DenylistCaseInsensitive(t *testing.T) {
	err := CheckBashCommand("RM -rf /", BashLimited)
	if err == nil {
		t.Error("expected RM to be denied (case insensitive)")
	}
}

func TestContainsDeniedPrefix(t *testing.T) {
	prefix, found := containsDeniedPrefix("go test ./...", testPrefixes)
	if !found {
		t.Error("expected to find go test prefix")
	}
	if prefix != "go test" {
		t.Errorf("expected 'go test', got %q", prefix)
	}

	_, found = containsDeniedPrefix("rm -rf /", testPrefixes)
	if found {
		t.Error("expected not to find rm in test prefixes")
	}
}

func TestToolDeniedErrorFormat(t *testing.T) {
	err := ToolDeniedError("inspect", "write")
	if !strings.Contains(err, "write") {
		t.Errorf("error should mention tool name: %s", err)
	}
	if !strings.Contains(err, "inspect") {
		t.Errorf("error should mention mode: %s", err)
	}
}

func TestToolAskErrorFormat(t *testing.T) {
	err := ToolAskError("build", "edit")
	if !strings.Contains(err, "approval") {
		t.Errorf("error should mention approval: %s", err)
	}
}
