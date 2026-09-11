package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	maxWebResponseBytes = 2 * 1024 * 1024
	webFetchTimeout     = 60 * time.Second
	webUserAgent        = "patcode/0.1 (+https://github.com/PatrykPanasiuk/patcode)"
)

type WebFetchArgs struct {
	URL    string `json:"url"`
	Format string `json:"format,omitempty"`
}

type WebSearchArgs struct {
	Query      string `json:"query"`
	NumResults int    `json:"num_results,omitempty"`
}

func WebFetchTool(workdir string) Tool {
	return Tool{
		Name:        "webfetch",
		Description: "Fetch a URL and return its content as markdown or plain text. Use for browsing web pages, reading documentation, API references, and articles.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch (http or https only).",
				},
				"format": map[string]any{
					"type":        "string",
					"description": "Output format: 'markdown' (default) or 'text'.",
					"enum":        []string{"markdown", "text"},
				},
			},
			"required": []string{"url"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var webArgs WebFetchArgs
			if err := json.Unmarshal(args, &webArgs); err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("invalid arguments: %v", err)}
			}
			if webArgs.URL == "" {
				return &ToolResult{Success: false, Error: "url is required"}
			}

			parsed, err := url.Parse(webArgs.URL)
			if err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("invalid url: %v", err)}
			}
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				return &ToolResult{Success: false, Error: "only http and https URLs are supported"}
			}

			format := webArgs.Format
			if format == "" {
				format = "markdown"
			}
			if format != "markdown" && format != "text" {
				return &ToolResult{Success: false, Error: "format must be 'markdown' or 'text'"}
			}

			client := &http.Client{Timeout: webFetchTimeout}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, webArgs.URL, nil)
			if err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("building request: %v", err)}
			}
			req.Header.Set("User-Agent", webUserAgent)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain,*/*")

			resp, err := client.Do(req)
			if err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("fetching url: %v", err)}
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("request failed with status %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode)),
				}
			}

			truncated := false
			limited := io.LimitReader(resp.Body, maxWebResponseBytes+1)
			raw, err := io.ReadAll(limited)
			if err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("reading response: %v", err)}
			}
			if len(raw) > maxWebResponseBytes {
				raw = raw[:maxWebResponseBytes]
				truncated = true
			}

			contentType := strings.ToLower(resp.Header.Get("Content-Type"))
			title, content := convertBody(raw, contentType, format)

			result := &ToolResult{Success: true}
			result.JSON, _ = json.Marshal(WebFetchResult{
				URL:         resp.Request.URL.String(),
				Title:       title,
				ContentType: contentType,
				Format:      format,
				Content:     content,
				Truncated:   truncated,
			})
			var out strings.Builder
			if title != "" {
				fmt.Fprintf(&out, "# %s\n\n", title)
			}
			out.WriteString(content)
			if truncated {
				out.WriteString("\n\n[output truncated at 2MB]")
			}
			result.Data = out.String()
			return result
		},
	}
}

type WebFetchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	ContentType string `json:"content_type"`
	Format      string `json:"format"`
	Content     string `json:"content"`
	Truncated   bool   `json:"truncated"`
}

func WebSearchTool(workdir string) Tool {
	return Tool{
		Name:        "websearch",
		Description: "Search the web for a query and return matching result titles, URLs, and snippets. Use to research topics, find documentation, and answer questions about the wider web.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query.",
				},
				"num_results": map[string]any{
					"type":        "integer",
					"description": "Number of results to return (default 5, max 10).",
				},
			},
			"required": []string{"query"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var searchArgs WebSearchArgs
			if err := json.Unmarshal(args, &searchArgs); err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("invalid arguments: %v", err)}
			}
			if searchArgs.Query == "" {
				return &ToolResult{Success: false, Error: "query is required"}
			}
			num := searchArgs.NumResults
			if num <= 0 || num > 10 {
				num = 5
			}

			var results []WebSearchResult
			var err error
			if apiKey := os.Getenv("SEARCH_API_KEY"); apiKey != "" {
				results, err = webSearchBrave(ctx, searchArgs.Query, num, apiKey)
			} else {
				results, err = webSearchDuckDuckGo(ctx, searchArgs.Query, num)
			}
			if err != nil {
				return &ToolResult{Success: false, Error: fmt.Sprintf("search failed: %v", err)}
			}

			result := &ToolResult{Success: true}
			result.JSON, _ = json.Marshal(results)
			var out strings.Builder
			if len(results) == 0 {
				out.WriteString("No results found.")
			} else {
				for i, r := range results {
					fmt.Fprintf(&out, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
				}
			}
			result.Data = out.String()
			return result
		},
	}
}

type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func webSearchDuckDuckGo(ctx context.Context, query string, numResults int) ([]WebSearchResult, error) {
	endpoint := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	client := &http.Client{Timeout: webFetchTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", webUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting search engine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search engine returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxWebResponseBytes))
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing search results: %w", err)
	}

	var results []WebSearchResult
	walkDuckDuckGoResults(doc, &results)
	if len(results) > numResults {
		results = results[:numResults]
	}
	return results, nil
}

func webSearchBrave(ctx context.Context, query string, numResults int, apiKey string) ([]WebSearchResult, error) {
	if numResults > 20 {
		numResults = 20
	}
	endpoint := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), numResults)
	client := &http.Client{Timeout: webFetchTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", webUserAgent)
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting search engine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search engine returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxWebResponseBytes))
	if err != nil {
		return nil, err
	}
	results, err := parseBraveResults(raw)
	if err != nil {
		return nil, err
	}
	if len(results) > numResults {
		results = results[:numResults]
	}
	return results, nil
}

type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func parseBraveResults(raw []byte) ([]WebSearchResult, error) {
	var parsed braveResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parsing search results: %w", err)
	}
	var results []WebSearchResult
	for _, r := range parsed.Web.Results {
		results = append(results, WebSearchResult{
			Title:   strings.TrimSpace(htmlStripTags(r.Title)),
			URL:     r.URL,
			Snippet: strings.TrimSpace(htmlStripTags(r.Description)),
		})
	}
	return results, nil
}

func htmlStripTags(s string) string {
	if s == "" {
		return s
	}
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}
	return nodeText(doc)
}

func walkDuckDuckGoResults(n *html.Node, results *[]WebSearchResult) {
	if n.Type == html.ElementNode && n.Data == "a" {
		var href string
		for _, attr := range n.Attr {
			if attr.Key == "href" {
				href = attr.Val
				break
			}
		}
		if strings.Contains(href, "uddg=") {
			if u, err := url.Parse(href); err == nil {
				if decoded := u.Query().Get("uddg"); decoded != "" {
					href = decoded
				}
			} else {
				href = ""
			}
		}
		if href != "" && strings.HasPrefix(href, "http") {
			title := strings.TrimSpace(nodeText(n))
			if title != "" {
				snippet := ""
				if sib := n.Parent; sib != nil && sib.Parent != nil {
					snippet = strings.TrimSpace(nodeText(sib.Parent))
				}
				*results = append(*results, WebSearchResult{
					Title:   title,
					URL:     href,
					Snippet: snippet,
				})
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkDuckDuckGoResults(c, results)
	}
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	collectText(n, &b)
	return b.String()
}

func convertBody(raw []byte, contentType, format string) (string, string) {
	ct := contentType
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = ct[:idx]
	}
	ct = strings.TrimSpace(ct)

	switch {
	case strings.Contains(ct, "html"), strings.Contains(ct, "xhtml"):
		doc, err := html.Parse(bytes.NewReader(raw))
		if err != nil {
			return "", strings.TrimSpace(string(raw))
		}
		title := extractTitle(doc)
		if format == "text" {
			return title, htmlToText(doc)
		}
		return title, htmlToMarkdown(doc)
	case strings.Contains(ct, "json"):
		var pretty bytes.Buffer
		if json.Indent(&pretty, raw, "", "  ") == nil {
			return "", pretty.String()
		}
		return "", strings.TrimSpace(string(raw))
	default:
		return "", strings.TrimSpace(string(raw))
	}
}

func extractTitle(doc *html.Node) string {
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" {
			title = strings.TrimSpace(nodeText(n))
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return title
}

func htmlToText(doc *html.Node) string {
	var b strings.Builder
	renderTextNode(doc, &b)
	return strings.TrimSpace(b.String())
}

func renderTextNode(n *html.Node, b *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		b.WriteString(n.Data)
	case html.ElementNode:
		switch n.Data {
		case "script", "style", "noscript", "head", "iframe":
			return
		case "br", "hr":
			b.WriteString("\n")
		case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote":
			b.WriteString("\n")
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderTextNode(c, b)
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "p", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote":
			b.WriteString("\n")
		}
	}
}

func htmlToMarkdown(doc *html.Node) string {
	var b strings.Builder
	renderMarkdownNode(doc, &b)
	return strings.TrimSpace(b.String())
}

func renderMarkdownNode(n *html.Node, b *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		b.WriteString(n.Data)
		return
	case html.CommentNode:
		return
	case html.ElementNode:
		renderMarkdownElement(n, b)
		return
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderMarkdownNode(c, b)
		}
	}
}

func renderMarkdownElement(n *html.Node, b *strings.Builder) {
	switch n.Data {
	case "script", "style", "noscript", "head", "iframe", "meta", "link":
		return
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		fmt.Fprintf(b, "\n%s ", strings.Repeat("#", level))
		renderMarkdownChildren(n, b)
		b.WriteString("\n")
	case "p":
		b.WriteString("\n")
		renderMarkdownChildren(n, b)
		b.WriteString("\n")
	case "a":
		href := attrVal(n, "href")
		b.WriteString("[")
		renderMarkdownChildren(n, b)
		b.WriteString("](")
		b.WriteString(href)
		b.WriteString(")")
	case "strong", "b":
		b.WriteString("**")
		renderMarkdownChildren(n, b)
		b.WriteString("**")
	case "em", "i":
		b.WriteString("*")
		renderMarkdownChildren(n, b)
		b.WriteString("*")
	case "code":
		b.WriteString("`")
		b.WriteString(nodeText(n))
		b.WriteString("`")
	case "pre":
		b.WriteString("\n```\n")
		b.WriteString(nodeText(n))
		b.WriteString("\n```\n")
	case "ul", "ol":
		b.WriteString("\n")
		renderMarkdownChildren(n, b)
		b.WriteString("\n")
	case "li":
		b.WriteString("- ")
		renderMarkdownChildren(n, b)
		b.WriteString("\n")
	case "blockquote":
		b.WriteString("\n> ")
		renderMarkdownChildren(n, b)
		b.WriteString("\n")
	case "br":
		b.WriteString("\n")
	case "img":
		b.WriteString("[image: ")
		b.WriteString(attrVal(n, "alt"))
		b.WriteString("](")
		b.WriteString(attrVal(n, "src"))
		b.WriteString(")")
	case "table":
		b.WriteString("\n")
		renderMarkdownChildren(n, b)
		b.WriteString("\n")
	case "tr":
		b.WriteString("| ")
		renderMarkdownChildren(n, b)
		b.WriteString("\n")
	case "th", "td":
		renderMarkdownChildren(n, b)
		b.WriteString(" | ")
	case "div", "span", "section", "article", "main", "header", "footer", "nav", "form", "button", "body", "html":
		renderMarkdownChildren(n, b)
	default:
		renderMarkdownChildren(n, b)
	}
}

func renderMarkdownChildren(n *html.Node, b *strings.Builder) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderMarkdownNode(c, b)
	}
}

func attrVal(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func collectText(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "noscript", "head":
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, b)
	}
}
