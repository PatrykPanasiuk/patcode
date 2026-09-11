package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfinePathRelative(t *testing.T) {
	root := t.TempDir()
	got, err := confinePath(root, "src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "src/main.go") {
		t.Errorf("got %q, want %q", got, filepath.Join(root, "src/main.go"))
	}
}

func TestConfinePathEscapes(t *testing.T) {
	root := t.TempDir()
	_, err := confinePath(root, "../outside.txt")
	if err == nil {
		t.Fatal("expected error for path escaping root")
	}
}

func TestConfinePathSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("topsecret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err := confinePath(root, "link.txt")
	if err == nil {
		t.Fatal("expected error reading symlink that escapes root")
	}
}

func TestConfinePathSymlinkDirEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	dir := filepath.Join(root, "sub")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err := confinePath(root, "sub/escape/victim.txt")
	if err == nil {
		t.Fatal("expected error writing through escaping symlink directory")
	}
}

func TestConfinePathWithinRootSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real-file.txt")
	if err := os.WriteFile(target, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	got, err := confinePath(root, "alias")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "alias") {
		t.Errorf("got %q", got)
	}
}

func TestConfinePathWriteNewFile(t *testing.T) {
	root := t.TempDir()
	got, err := confinePath(root, "new-cmd/run.sh")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "new-cmd/run.sh") {
		t.Errorf("got %q", got)
	}
}
