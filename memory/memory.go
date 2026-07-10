package memory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const memoryProject = "patcode-memory"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b[a-zA-Z0-9_-]{0,32}_(?:sk|pk|rk|mk)_[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`\b(?:sk|pk|rk)_live_[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`\b(?:sk|pk|rk)_test_[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`\bwhsec_[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`),
	regexp.MustCompile(`\b(?:mka|mk)_[A-Za-z0-9]{10,}\b`),
	regexp.MustCompile(`\b(?:eyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9._-]{10,}\.[a-zA-Z0-9._-]{10,})\b`),
}

var secretMarkers = []string{
	"sunny_sk_",
	"sunny_pk_",
	"pk_live_",
	"sk_live_",
	"pk_test_",
	"sk_test_",
	"whsec_",
	"ghp_",
	"gho_",
	"ghu_",
	"ghs_",
	"ghr_",
	"xoxb-",
	"xoxp-",
	"mka_",
	"mk_",
}

type Options struct {
	OutputDir         string
	CodexRoot         string
	OpenCodeRoot      string
	CodexHistoryPath   string
	OpenCodePromptPath string
	RagRoot           string
	RedactSecrets     bool
}

type Stats struct {
	SessionDocs   int `json:"session_docs"`
	PromptDocs    int `json:"prompt_docs"`
	Examples      int `json:"examples"`
	CorpusFiles   int `json:"corpus_files"`
	DatasetLines  int `json:"dataset_lines"`
}

type trainMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type sessionDoc struct {
	Source     string         `json:"source"`
	SourceFile string         `json:"source_file"`
	SessionID  string         `json:"session_id,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	CWD        string         `json:"cwd,omitempty"`
	System     string         `json:"system,omitempty"`
	Messages   []trainMessage `json:"messages"`
}

type promptItem struct {
	SessionID string `json:"session_id,omitempty"`
	TS        int64  `json:"ts,omitempty"`
	Text      string `json:"text"`
}

type promptDoc struct {
	Source     string       `json:"source"`
	SourceFile string       `json:"source_file"`
	Items      []promptItem `json:"items"`
}

type sessionMeta struct {
	ID        string
	Timestamp string
	CWD       string
}

func DefaultOptions(workspaceRoot string) Options {
	home, _ := os.UserHomeDir()
	parent := filepath.Dir(workspaceRoot)
	ragRoot := filepath.Join(parent, "rag")
	return Options{
		OutputDir:          filepath.Join(home, ".patcode", "memory"),
		CodexRoot:          filepath.Join(home, ".codex", "sessions"),
		OpenCodeRoot:       filepath.Join(home, ".local", "share", "opencode", "tool-output"),
		CodexHistoryPath:   filepath.Join(home, ".codex", "history.jsonl"),
		OpenCodePromptPath: filepath.Join(home, ".local", "state", "opencode", "prompt-history.jsonl"),
		RagRoot:            ragRoot,
		RedactSecrets:      true,
	}
}

func Export(opts Options) (Stats, error) {
	outDir, err := expandHome(opts.OutputDir)
	if err != nil {
		return Stats{}, err
	}
	if err := os.RemoveAll(outDir); err != nil {
		return Stats{}, fmt.Errorf("clearing output dir: %w", err)
	}
	corpusDir := filepath.Join(outDir, "corpus")
	sessionsDir := filepath.Join(corpusDir, "sessions")
	promptsDir := filepath.Join(corpusDir, "prompts")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return Stats{}, fmt.Errorf("creating corpus dir: %w", err)
	}
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		return Stats{}, fmt.Errorf("creating prompts dir: %w", err)
	}

	datasetPath := filepath.Join(outDir, "dataset.jsonl")
	datasetFile, err := os.Create(datasetPath)
	if err != nil {
		return Stats{}, fmt.Errorf("creating dataset file: %w", err)
	}
	defer datasetFile.Close()

	stats := Stats{}
	systemPrompt := "You are patcode, a local coding assistant. Be concise, pragmatic, direct, and use the user's language."

	if opts.CodexRoot != "" {
		if err := exportSessionRoot(opts.CodexRoot, "codex", sessionsDir, systemPrompt, datasetFile, &stats, opts.RedactSecrets); err != nil {
			return Stats{}, err
		}
	}
	if opts.OpenCodeRoot != "" {
		if err := exportOpenCodeRoot(opts.OpenCodeRoot, "opencode", sessionsDir, systemPrompt, datasetFile, &stats, opts.RedactSecrets); err != nil {
			return Stats{}, err
		}
	}
	if opts.CodexHistoryPath != "" {
		if err := exportPromptHistory(opts.CodexHistoryPath, "codex-history", promptsDir, &stats, opts.RedactSecrets); err != nil {
			return Stats{}, err
		}
	}
	if opts.OpenCodePromptPath != "" {
		if err := exportPromptHistory(opts.OpenCodePromptPath, "opencode-prompt-history", promptsDir, &stats, opts.RedactSecrets); err != nil {
			return Stats{}, err
		}
	}

	manifest := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"output_dir":   outDir,
		"corpus_dir":   corpusDir,
		"dataset_path": datasetPath,
		"stats":        stats,
		"sources": map[string]string{
			"codex_root":           opts.CodexRoot,
			"opencode_root":         opts.OpenCodeRoot,
			"codex_history":         opts.CodexHistoryPath,
			"opencode_prompt_history": opts.OpenCodePromptPath,
		},
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Stats{}, fmt.Errorf("marshalling manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return Stats{}, fmt.Errorf("writing manifest: %w", err)
	}

	return stats, nil
}

func Ingest(ctx context.Context, ragRoot, corpusDir string) error {
	ragRoot, err := expandHome(ragRoot)
	if err != nil {
		return err
	}
	python := firstNonEmpty(
		os.Getenv("RAG_PYTHON"),
		"python3",
		filepath.Join(ragRoot, "venv", "bin", "python"),
	)

	script := `
import sys
from pathlib import Path

root = Path(sys.argv[1])
sys.path.insert(0, str(root))

from rag.ingest import ingest_directory

total = ingest_directory(Path(sys.argv[2]), project=sys.argv[3])
print(total)
`
	cmd := exec.CommandContext(ctx, python, "-c", script, ragRoot, corpusDir, memoryProject)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("ingest failed: %s", strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

func MemoryProject() string {
	return memoryProject
}

func exportSessionRoot(root, source, outDir, systemPrompt string, datasetFile *os.File, stats *Stats, redact bool) error {
	absRoot, err := expandHome(root)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", absRoot, err)
	}

	var files []string
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking %s: %w", absRoot, err)
	}
	sort.Strings(files)

	for _, path := range files {
		doc, ok, err := parseSessionFile(path, source, systemPrompt, redact)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			rel = filepath.Base(path)
		}
		outPath := filepath.Join(outDir, filepath.Dir(rel), strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))+".json")
		if err := writeJSONFile(outPath, doc); err != nil {
			return err
		}
		stats.SessionDocs++
		stats.CorpusFiles++
		if len(doc.Messages) > 1 {
			if err := writeDatasetLine(datasetFile, doc); err != nil {
				return err
			}
			stats.Examples++
			stats.DatasetLines++
		}
	}

	return nil
}

func exportOpenCodeRoot(root, source, outDir, systemPrompt string, datasetFile *os.File, stats *Stats, redact bool) error {
	absRoot, err := expandHome(root)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", absRoot, err)
	}

	var files []string
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking %s: %w", absRoot, err)
	}
	sort.Strings(files)

	for _, path := range files {
		doc, ok, err := parseSessionFile(path, source, systemPrompt, redact)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			rel = filepath.Base(path)
		}
		outPath := filepath.Join(outDir, source, filepath.Dir(rel), strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))+".json")
		if err := writeJSONFile(outPath, doc); err != nil {
			return err
		}
		stats.SessionDocs++
		stats.CorpusFiles++
		if len(doc.Messages) > 1 {
			if err := writeDatasetLine(datasetFile, doc); err != nil {
				return err
			}
			stats.Examples++
			stats.DatasetLines++
		}
	}

	return nil
}

func exportPromptHistory(path, source, outDir string, stats *Stats, redact bool) error {
	absPath, err := expandHome(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", absPath, err)
	}

	items, err := parsePromptFile(absPath, redact)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	doc := promptDoc{
		Source:     source,
		SourceFile: absPath,
		Items:      items,
	}
	outPath := filepath.Join(outDir, source+".json")
	if err := writeJSONFile(outPath, doc); err != nil {
		return err
	}
	stats.PromptDocs++
	stats.CorpusFiles++
	return nil
}

func parseSessionFile(path, source, systemPrompt string, redact bool) (sessionDoc, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return sessionDoc{}, false, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	doc := sessionDoc{
		Source:     source,
		SourceFile: path,
		System:     systemPrompt,
	}

	var haveAny bool
	var hasAssistant bool
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return sessionDoc{}, false, fmt.Errorf("reading %s: %w", path, err)
		}
		line = strings.TrimSpace(line)
		if line != "" {
			var raw map[string]any
			if json.Unmarshal([]byte(line), &raw) == nil {
				if meta, ok := extractSessionMeta(raw); ok {
					if doc.SessionID == "" {
						doc.SessionID = meta.ID
					}
					if doc.CreatedAt == "" {
						doc.CreatedAt = meta.Timestamp
					}
					if doc.CWD == "" {
						doc.CWD = meta.CWD
					}
				}
				if msg, ok := extractMessage(raw, redact); ok {
					doc.Messages = appendMessage(doc.Messages, msg)
					haveAny = true
					if msg.Role == "assistant" {
						hasAssistant = true
					}
				}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}

	if doc.SessionID == "" {
		doc.SessionID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if !haveAny {
		return sessionDoc{}, false, nil
	}
	if !hasAssistant {
		return sessionDoc{}, false, nil
	}
	return doc, true, nil
}

func parsePromptFile(path string, redact bool) ([]promptItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	var items []promptItem
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		line = strings.TrimSpace(line)
		if line != "" {
			var raw map[string]any
			if json.Unmarshal([]byte(line), &raw) == nil {
				text := firstNonEmpty(stringFromMap(raw, "text"), stringFromMap(raw, "input"))
				if redact {
					text = sanitizeText(text)
				}
				if text != "" {
					items = append(items, promptItem{
						SessionID: stringFromMap(raw, "session_id"),
						TS:        int64FromMap(raw, "ts"),
						Text:      text,
					})
				}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return items, nil
}

func extractSessionMeta(raw map[string]any) (sessionMeta, bool) {
	if stringFromMap(raw, "type") != "session_meta" {
		return sessionMeta{}, false
	}
	payload, _ := raw["payload"].(map[string]any)
	if payload == nil {
		return sessionMeta{}, false
	}
	return sessionMeta{
		ID:        stringFromMap(payload, "id"),
		Timestamp: firstNonEmpty(stringFromMap(payload, "timestamp"), stringFromMap(raw, "timestamp")),
		CWD:       stringFromMap(payload, "cwd"),
	}, true
}

func extractMessage(raw map[string]any, redact bool) (trainMessage, bool) {
	if stringFromMap(raw, "type") != "response_item" {
		return trainMessage{}, false
	}
	payload, _ := raw["payload"].(map[string]any)
	if payload == nil {
		return trainMessage{}, false
	}
	if stringFromMap(payload, "type") != "message" {
		return trainMessage{}, false
	}
	role := stringFromMap(payload, "role")
	if role != "user" && role != "assistant" {
		return trainMessage{}, false
	}
	content := extractText(payload["content"])
	if redact {
		content = sanitizeText(content)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return trainMessage{}, false
	}
	return trainMessage{Role: role, Content: content}, true
}

func extractText(v any) string {
	var parts []string
	gatherText(v, &parts)
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func gatherText(v any, parts *[]string) {
	switch x := v.(type) {
	case nil:
		return
	case string:
		if strings.TrimSpace(x) != "" {
			*parts = append(*parts, x)
		}
	case []any:
		for _, item := range x {
			gatherText(item, parts)
		}
	case map[string]any:
		if text, ok := x["text"].(string); ok && strings.TrimSpace(text) != "" {
			*parts = append(*parts, text)
			return
		}
		for _, key := range []string{"content", "input_text", "output_text", "parts"} {
			if value, ok := x[key]; ok {
				gatherText(value, parts)
			}
		}
	default:
		return
	}
}

func appendMessage(messages []trainMessage, next trainMessage) []trainMessage {
	next.Content = strings.TrimSpace(next.Content)
	if next.Content == "" {
		return messages
	}
	if len(messages) > 0 && messages[len(messages)-1].Role == next.Role {
		messages[len(messages)-1].Content += "\n\n" + next.Content
		return messages
	}
	return append(messages, next)
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating dir for %s: %w", path, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling %s: %w", path, err)
	}
	return os.WriteFile(path, data, 0o644)
}

func writeDatasetLine(file *os.File, doc sessionDoc) error {
	line, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshalling dataset line: %w", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("writing dataset line: %w", err)
	}
	return nil
}

func redactSecrets(text string) string {
	out := text
	for _, re := range secretPatterns {
		out = re.ReplaceAllString(out, "[REDACTED_SECRET]")
	}
	return out
}

func sanitizeText(text string) string {
	out := redactSecrets(text)
	lower := strings.ToLower(out)
	for _, marker := range secretMarkers {
		if strings.Contains(lower, marker) {
			return "[REDACTED_SECRET]"
		}
	}
	for _, re := range secretPatterns {
		if re.MatchString(out) {
			return "[REDACTED_SECRET]"
		}
	}
	return out
}

func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		switch x := v.(type) {
		case string:
			return x
		case fmt.Stringer:
			return x.String()
		}
	}
	return ""
}

func int64FromMap(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func expandHome(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		if strings.HasPrefix(path, "~/") {
			return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
		}
	}
	return filepath.Clean(path), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
