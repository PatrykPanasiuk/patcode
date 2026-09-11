package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestWebFetchTool_HTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><title>Test Page</title></head>
<body>
<script>var dropp = 1;</script>
<h1>Hello World</h1>
<p>Some <strong>bold</strong> text and a <a href="https://example.com">link</a>.</p>
<pre><code>func main(){}</code></pre>
</body></html>`))
	}))
	defer srv.Close()

	tool := WebFetchTool("/tmp")
	args, _ := json.Marshal(map[string]string{"url": srv.URL})
	result := tool.Execute(context.Background(), args)
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if !strings.Contains(result.Data, "Test Page") {
		t.Errorf("expected title in output, got: %q", result.Data)
	}
	if !strings.Contains(result.Data, "Hello World") {
		t.Errorf("expected heading in output, got: %q", result.Data)
	}
	if !strings.Contains(result.Data, "bold") {
		t.Errorf("expected bold text in output, got: %q", result.Data)
	}
	if strings.Contains(result.Data, "dropp") {
		t.Errorf("expected script content to be stripped, got: %q", result.Data)
	}
}

func TestWebFetchTool_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("just some text"))
	}))
	defer srv.Close()

	tool := WebFetchTool("/tmp")
	args, _ := json.Marshal(map[string]string{"url": srv.URL})
	result := tool.Execute(context.Background(), args)
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if result.Data != "just some text" {
		t.Errorf("expected raw text, got: %q", result.Data)
	}
}

func TestWebFetchTool_RejectsNonHTTP(t *testing.T) {
	tool := WebFetchTool("/tmp")
	args, _ := json.Marshal(map[string]string{"url": "file:///etc/passwd"})
	result := tool.Execute(context.Background(), args)
	if result.Success {
		t.Fatal("expected failure for non-http url")
	}
	if !strings.Contains(result.Error, "http") {
		t.Errorf("expected scheme error, got: %s", result.Error)
	}
}

func TestWebFetchTool_RejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	tool := WebFetchTool("/tmp")
	args, _ := json.Marshal(map[string]string{"url": srv.URL})
	result := tool.Execute(context.Background(), args)
	if result.Success {
		t.Fatal("expected failure for 404")
	}
	if !strings.Contains(result.Error, "404") {
		t.Errorf("expected 404 in error, got: %s", result.Error)
	}
}

func TestConvertBody_MarkdownLinks(t *testing.T) {
	raw := []byte(`<html><body><p>See <a href="https://x.dev">docs</a></p></body></html>`)
	title, out := convertBody(raw, "text/html", "markdown")
	if title != "" {
		t.Errorf("expected empty title, got %q", title)
	}
	if !strings.Contains(out, "[docs](https://x.dev)") {
		t.Errorf("expected markdown link, got: %q", out)
	}
}

func TestWebSearchArgs_RequiresQuery(t *testing.T) {
	tool := WebSearchTool("/tmp")
	result := tool.Execute(context.Background(), nil)
	if result.Success {
		t.Fatal("expected failure for nil args")
	}
	args, _ := json.Marshal(map[string]string{})
	result = tool.Execute(context.Background(), args)
	if result.Success {
		t.Fatal("expected failure for empty query")
	}
}

func TestWalkDuckDuckGoResults(t *testing.T) {
	body := `<html><body><div class="result">
<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fdocs&amp;rut=x">Example Docs</a>
<div class="result__snippet">Some snippet about examples</div>
</div></body></html>`
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var results []WebSearchResult
	walkDuckDuckGoResults(doc, &results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Example Docs" {
		t.Errorf("expected title, got %q", results[0].Title)
	}
	if results[0].URL != "https://example.com/docs" {
		t.Errorf("expected decoded url, got %q", results[0].URL)
	}
}

func TestParseBraveResults(t *testing.T) {
	body := []byte(`{"web":{"results":[
		{"title":"First &amp; Second","url":"https://example.com/1","description":"Short <b>snippet</b> one"},
		{"title":"Two","url":"https://example.com/2","description":"Snippet two"}
	]}}`)
	results, err := parseBraveResults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "First & Second" {
		t.Errorf("expected decoded entity, got %q", results[0].Title)
	}
	if results[0].Snippet != "Short snippet one" {
		t.Errorf("expected stripped tags, got %q", results[0].Snippet)
	}
	if results[1].URL != "https://example.com/2" {
		t.Errorf("unexpected url: %q", results[1].URL)
	}
}

func TestParseBraveResults_Empty(t *testing.T) {
	results, err := parseBraveResults([]byte(`{"web":{"results":[]}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}

func TestParseBraveResults_BadJSON(t *testing.T) {
	if _, err := parseBraveResults([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid json")
	}
}
