package tools

import (
	"fmt"
	"strings"
)

// shellWords splits a command string into argv tokens following the common
// POSIX shell quoting rules: single quotes preserve everything literally,
// double quotes allow backslash escapes, and a bare backslash escapes the
// following character. Unlike a real shell it performs no metacharacter
// interpretation, glob expansion, or environment expansion.
func shellWords(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inToken := false

	i := 0
	for i < len(s) {
		ch := s[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			if inToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inToken = false
			}
			i++

		case ch == '\'':
			inToken = true
			j := i + 1
			for j < len(s) && s[j] != '\'' {
				cur.WriteByte(s[j])
				j++
			}
			if j >= len(s) {
				return nil, fmt.Errorf("unterminated single quote")
			}
			i = j + 1

		case ch == '"':
			inToken = true
			j := i + 1
			for j < len(s) {
				if s[j] == '"' {
					break
				}
				if s[j] == '\\' && j+1 < len(s) {
					next := s[j+1]
					switch next {
					case '"', '\\', '$', '`':
						cur.WriteByte(next)
					case 'n':
						cur.WriteByte('\n')
					case 't':
						cur.WriteByte('\t')
					default:
						cur.WriteByte('\\')
						cur.WriteByte(next)
					}
					j += 2
					continue
				}
				cur.WriteByte(s[j])
				j++
			}
			if j >= len(s) {
				return nil, fmt.Errorf("unterminated double quote")
			}
			i = j + 1

		case ch == '\\':
			inToken = true
			if i+1 < len(s) {
				cur.WriteByte(s[i+1])
				i += 2
			} else {
				cur.WriteByte('\\')
				i++
			}

		default:
			inToken = true
			cur.WriteByte(ch)
			i++
		}
	}

	if inToken {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}

// unsafeShellChars is the set of characters whose presence outside quotes
// would change command semantics in a real shell. Executing them as raw
// argv is either misleading (e.g. pipes/redirects silently ignored) or a
// command-injection vector, so they are refused unless the caller opts
// back into shell passthrough.
const unsafeShellChars = "|&;<>$`()"

// hasUnsafeShellChars reports whether s contains shell metacharacters
// outside of quoted regions.
func hasUnsafeShellChars(s string) bool {
	single := false
	double := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		switch {
		case single:
			if ch == '\'' {
				single = false
			}
			continue
		case double:
			if ch == '"' {
				double = false
			}
			continue
		case ch == '\'':
			single = true
			continue
		case ch == '"':
			double = true
			continue
		case strings.IndexByte(unsafeShellChars, ch) >= 0:
			return true
		}
	}
	return false
}
