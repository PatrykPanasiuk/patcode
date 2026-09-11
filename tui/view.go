package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"patcode/session"
	"patcode/version"
)

func (m *model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	header := m.renderHeader()
	messages := m.renderMessages()
	composer := m.renderComposer()
	status := m.renderStatusBar()
	modeBox := m.renderModeBox()

	footer := composer + "\n" + status + "\n" + modeBox
	used := lipgloss.Height(header+"\n") + lipgloss.Height(messages) + lipgloss.Height(footer)
	spacer := strings.Repeat("\n", max(0, m.height-used))

	return header + "\n" + messages + spacer + footer
}

func (m *model) renderHeader() string {
	if len(m.messages) > 0 {
		return ""
	}
	return dimStyle.Render(asciiArt)
}

func (m *model) renderComposer() string {
	if m.showHelp {
		return m.renderHelp()
	}

	prompt := "> "
	if m.loading {
		prompt = "... "
	}
	if m.session.GetMode() == session.ModeShell {
		prompt = "$ "
	}

	value := m.textarea.Value()
	if value == "" {
		value = dimStyle.Render("message... (/? for help)")
	} else {
		value = lipgloss.NewStyle().Foreground(textColor).Render(value)
	}
	inputView := fmt.Sprintf("%s%s", dimStyle.Render(prompt), value)
	return inputView
}

func (m *model) renderMessages() string {
	if len(m.messages) == 0 {
		return ""
	}

	var rendered []string
	for _, msg := range m.messages {
		var style lipgloss.Style
		prefix := ""
		switch msg.Role {
		case "user":
			style = userMsgStyle
			prefix = ">"
		case "assistant":
			style = assistantMsgStyle
			prefix = ""
		case "tool":
			style = toolMsgStyle
			prefix = "tool"
		case "system":
			style = systemMsgStyle
			prefix = "sys"
		}

		content := m.renderRichText(msg.Content)
		if msg.Role == "assistant" && m.loading && msg == m.messages[len(m.messages)-1] {
			content += " ▊"
		}

		if prefix != "" {
			rendered = append(rendered, style.Render(
				style.Width(m.width-4).Render(
					fmt.Sprintf("%s %s", dimStyle.Render(prefix), content),
				),
			))
		} else {
			rendered = append(rendered, style.Render(
				style.Width(m.width-4).Render(content),
			))
		}
	}

	m.viewport.SetContent(strings.Join(rendered, "\n"))
	return m.viewport.View()
}

func (m *model) renderHelp() string {
	help := lipgloss.NewStyle().
		Width(m.width-4).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(dimColor).
		Padding(1, 2).
		Render(
			dimStyle.Render("Commands:") + "\n" +
				"  /help      toggle help\n" +
				"  /plan      switch to plan mode\n" +
				"  /ask       switch to ask mode\n" +
				"  /build     switch to build mode\n" +
				"  /shell     switch to shell mode\n" +
				"  /mode      show mode\n" +
				"  /clear     clear chat\n" +
				"  /session   session info\n" +
				"  /exit      quit\n\n" +
				dimStyle.Render("Keys:") + "\n" +
				"  Ctrl+C     cancel\n" +
				"  Ctrl+L     clear\n" +
				"  PgUp/PgDn  scroll\n" +
				"  Tab        cycle mode\n\n" +
				dimStyle.Render("Esc to close"),
		)
	return help
}

func (m *model) renderModeBox() string {
	mode := m.session.GetMode()
	label := strings.ToUpper(string(mode))

	box := renderModeBoxStyle(mode).Render(" " + label + " ")
	return box
}

func (m *model) renderStatusBar() string {
	modelLabel := m.providerModel
	if modelLabel == "" {
		modelLabel = "none"
	}

	statusColor := dimColor
	statusDot := "●"
	if m.providerReady {
		statusColor = accent2Color
		statusDot = "●"
	}

	left := lipgloss.NewStyle().Foreground(dimColor).Render(version.Name + " v" + version.Version + " · " + m.providerType)
	right := lipgloss.NewStyle().Foreground(statusColor).Render(statusDot + " " + modelLabel)

	return lipgloss.NewStyle().
		Width(m.width - 4).
		MaxWidth(m.width - 4).
		Render(left + right)
}

func renderModeBoxStyle(mode session.Mode) lipgloss.Style {
	switch mode {
	case session.ModeAsk:
		return modeAskStyle
	case session.ModeInspect:
		return modeInspectStyle
	case session.ModePlan:
		return modePlanStyle
	case session.ModeReview:
		return modeReviewStyle
	case session.ModeAudit:
		return modeAuditStyle
	case session.ModePatch:
		return modePatchStyle
	case session.ModeBuild:
		return modeBuildStyle
	case session.ModeFix:
		return modeFixStyle
	case session.ModeRefactor:
		return modeRefactorStyle
	case session.ModeScaffold:
		return modeScaffoldStyle
	case session.ModeTest:
		return modeTestStyle
	case session.ModeCI:
		return modeCIStyle
	case session.ModeShell:
		return modeShellStyle
	default:
		return modeActiveFrameStyle
	}
}

func (m *model) renderRichText(s string) string {
	return renderCodeBlocks(s)
}

func renderCodeBlocks(text string) string {
	re := regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\\n(.*?)```")
	return re.ReplaceAllStringFunc(text, func(block string) string {
		m := re.FindStringSubmatch(block)
		if len(m) != 3 {
			return block
		}
		lang := strings.ToLower(strings.TrimSpace(m[1]))
		code := highlightCode(m[2], lang)
		return codeFenceStyle.Render(code)
	})
}

func highlightCode(code, lang string) string {
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		lines[i] = highlightLine(line, lang)
	}
	return strings.Join(lines, "\n")
}

func highlightLine(line, lang string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
		return codeCommentStyle.Render(line)
	}

	words := strings.Fields(line)
	for i, w := range words {
		switch lang {
		case "go", "js", "ts", "tsx", "jsx", "py", "python", "sh", "bash":
			if isKeyword(w, lang) {
				words[i] = codeKeywordStyle.Render(w)
			}
		}
	}
	rendered := strings.Join(words, " ")
	if strings.Contains(line, "\"") {
		rendered = quoteStrings(rendered)
	}
	return rendered
}

func isKeyword(word, lang string) bool {
	keywords := map[string]map[string]bool{
		"go": {
			"func": true, "package": true, "import": true, "return": true, "if": true, "else": true,
			"for": true, "range": true, "struct": true, "type": true, "var": true, "const": true,
			"switch": true, "case": true, "defer": true, "go": true, "select": true,
		},
		"js": {"function": true, "return": true, "const": true, "let": true, "var": true, "if": true, "else": true},
		"ts": {"function": true, "return": true, "const": true, "let": true, "var": true, "if": true, "else": true, "type": true, "interface": true},
		"py": {"def": true, "return": true, "if": true, "else": true, "for": true, "in": true, "class": true, "import": true},
		"sh": {"if": true, "then": true, "fi": true, "for": true, "do": true, "done": true, "echo": true, "cd": true, "ls": true, "pwd": true, "touch": true},
	}
	set := keywords[lang]
	if set == nil {
		return false
	}
	return set[word]
}

func quoteStrings(s string) string {
	re := regexp.MustCompile(`"([^"\\]|\\.)*"`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		return codeStringStyle.Render(match)
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
