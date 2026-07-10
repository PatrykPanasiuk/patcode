package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	dimColor      = lipgloss.Color("#5c6670")
	textColor     = lipgloss.Color("#dfe7ef")
	accentColor   = lipgloss.Color("#0f62fe")
	accent2Color  = lipgloss.Color("#42be65")
	warningColor  = lipgloss.Color("#f1c21b")
	errorColor    = lipgloss.Color("#da1e28")
	bgColor       = lipgloss.Color("#0f1419")
	surfaceColor  = lipgloss.Color("#161d24")
	codeBgColor   = lipgloss.Color("#101820")
)

var (
	userMsgStyle = lipgloss.NewStyle().
		Foreground(textColor).
		Padding(0, 1).
		MarginBottom(1)

	assistantMsgStyle = lipgloss.NewStyle().
		Foreground(textColor).
		Padding(0, 1).
		MarginBottom(1)

	toolMsgStyle = lipgloss.NewStyle().
		Foreground(accent2Color).
		Padding(0, 1).
		MarginBottom(1).
		Italic(true)

	systemMsgStyle = lipgloss.NewStyle().
		Foreground(dimColor).
		Padding(0, 1).
		MarginBottom(1).
		Italic(true)

	statusBarStyle = lipgloss.NewStyle().
		Background(surfaceColor).
		Foreground(textColor).
		Padding(0, 1).
		MarginTop(1)

	helpStyle = lipgloss.NewStyle().
		Foreground(dimColor).
		MarginTop(1)

	modeActiveFrameStyle = lipgloss.NewStyle().
		Background(accentColor).
		Foreground(textColor).
		Padding(0, 1)

	modePlanStyle = lipgloss.NewStyle().Foreground(textColor).Background(lipgloss.Color("#202a35")).Padding(0, 1)
	modeAskStyle = lipgloss.NewStyle().Foreground(textColor).Background(accentColor).Padding(0, 1)
	modeBuildStyle = lipgloss.NewStyle().Foreground(textColor).Background(accent2Color).Padding(0, 1)
	modeShellStyle = lipgloss.NewStyle().Foreground(textColor).Background(warningColor).Padding(0, 1)
	codeFenceStyle = lipgloss.NewStyle().Foreground(textColor).Background(codeBgColor)
	codeKeywordStyle = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	codeStringStyle = lipgloss.NewStyle().Foreground(accent2Color)
	codeCommentStyle = lipgloss.NewStyle().Foreground(dimColor).Italic(true)
)

var dimStyle = lipgloss.NewStyle().Foreground(dimColor)
