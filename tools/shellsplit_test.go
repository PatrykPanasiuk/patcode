package tools

import (
	"reflect"
	"testing"
)

func TestShellWordsBasics(t *testing.T) {
	got, err := shellWords("go test ./...")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"go", "test", "./..."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestShellWordsSingleQuotes(t *testing.T) {
	got, err := shellWords(`python -c 'print("hi")'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"python", "-c", `print("hi")`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestShellWordsUnterminatedQuote(t *testing.T) {
	if _, err := shellWords(`echo "abc`); err == nil {
		t.Error("expected error for unterminated quote")
	}
}

func TestShellWordsBackslashEscape(t *testing.T) {
	got, err := shellWords(`git log --oneline hello\ world`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"git", "log", "--oneline", "hello world"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestHasUnsafeShellChars(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"go test ./...", false},
		{"ls -la", false},
		{"echo 'a | b'", false}, // pipe inside quotes is safe
		{`echo "a | b"`, false}, // pipe inside double quotes is safe
		{"go test ./... | tee x", true},
		{"a && b", true},
		{"a; b", true},
		{"rm x > /dev/null", true},
		{"echo $HOME", true},
		{"cat < file", true},
	}
	for _, c := range cases {
		if got := hasUnsafeShellChars(c.in); got != c.want {
			t.Errorf("hasUnsafeShellChars(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
