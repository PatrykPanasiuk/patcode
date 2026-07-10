package ragbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Enabled       bool
	ProjectRoot   string
	DSN           string
	TopK          int
	MinQueryLen   int
	FilterProject  string
	PythonCommand string
}

type chunk struct {
	Project    string `json:"project"`
	SourcePath string `json:"source_path"`
	ChunkIndex int    `json:"chunk_index"`
	Total      int    `json:"total_chunks"`
	Content    string `json:"content"`
}

func DefaultConfig(projectRoot string) Config {
	return Config{
		Enabled:       os.Getenv("RAG_ENABLED") != "0",
		ProjectRoot:   projectRoot,
		DSN:           getenv("RAG_DSN", "postgresql://rag:rag@localhost:5432/rag"),
		TopK:          getenvInt("RAG_TOP_K", 5),
		MinQueryLen:   getenvInt("RAG_MIN_QUERY_LEN", 15),
		FilterProject: os.Getenv("RAG_FILTER_PROJECT"),
		PythonCommand: firstNonEmpty(os.Getenv("RAG_PYTHON"), filepath.Join(projectRoot, "rag", "venv", "bin", "python"), "python3"),
	}
}

func ContextForQuery(ctx context.Context, cfg Config, query string) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}

	if len(strings.TrimSpace(query)) < cfg.MinQueryLen || isMetaCommand(query) {
		return "", nil
	}

	chunks, err := retrieve(ctx, cfg, query)
	if err != nil || len(chunks) == 0 {
		return "", err
	}

	var blocks []string
	for _, c := range chunks {
		src := fmt.Sprintf("%s/%s", c.Project, c.SourcePath)
		if c.ChunkIndex > 0 {
			src += fmt.Sprintf(" (part %d/%d)", c.ChunkIndex+1, c.Total)
		}
		blocks = append(blocks, fmt.Sprintf("[Source: %s]\n%s", src, c.Content))
	}

	contextBlock := "\n\n=== LOCAL RAG CONTEXT ===\nUse the following retrieved fragments if relevant.\n\n" +
		strings.Join(blocks, "\n\n---\n\n") +
		"\n=== END LOCAL RAG CONTEXT ===\n"
	return contextBlock, nil
}

func retrieve(ctx context.Context, cfg Config, query string) ([]chunk, error) {
	script := `
import json, os, sys
root = os.path.abspath(sys.argv[1])
sys.path.insert(0, root)
from rag.retriever import retrieve
import psycopg2

query = sys.argv[2]
top_k = int(sys.argv[3])
project = sys.argv[4]
dsn = sys.argv[5]

os.environ["RAG_DSN"] = dsn
filters = {}
if project:
    filters["project"] = project

conn = psycopg2.connect(dsn)
try:
    rows = retrieve(query, conn=conn, filters=filters, final_k=top_k)
    print(json.dumps(rows))
finally:
    conn.close()
`

	cmd := exec.CommandContext(ctx, cfg.PythonCommand, "-c", script, cfg.ProjectRoot, query, strconv.Itoa(cfg.TopK), cfg.FilterProject, cfg.DSN)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("rag query failed: %s", strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}

	var rows []chunk
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		return nil, fmt.Errorf("decoding rag rows: %w", err)
	}
	return rows, nil
}

func isMetaCommand(text string) bool {
	prefixes := []string{"/", "rag:", "ingest:", "stats:", "clear:", "help", "status"}
	t := strings.TrimSpace(text)
	for _, p := range prefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

func getenv(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func getenvInt(name string, fallback int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
