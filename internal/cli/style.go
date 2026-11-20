package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Styling rules, applied consistently so the output reads as one tool:
//
//	success  green  ✓
//	pending  yellow ·
//	failure  red    ✗
//	label    dim
//	value    bold
//
// Colour is disabled when stdout is not a terminal, when NO_COLOR is set, or
// when TERM=dumb, so piping into a file or another program yields plain text.
var colorEnabled = detectColor()

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	// A character device means a terminal; a pipe or file does not.
	st, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

func style(s string, opts ...lipgloss.Style) string {
	if !colorEnabled {
		return s
	}
	st := lipgloss.NewStyle()
	for _, o := range opts {
		st = st.Inherit(o)
	}
	return st.Render(s)
}

var (
	stSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stError   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	stWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	stBold    = lipgloss.NewStyle().Bold(true)
	stTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	stHead    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
)

func ok(s string) string    { return style(s, stSuccess) }
func bad(s string) string   { return style(s, stError) }
func warn(s string) string  { return style(s, stWarn) }
func dim(s string) string   { return style(s, stDim) }
func bold(s string) string  { return style(s, stBold) }
func title(s string) string { return style(s, stTitle) }
func head(s string) string  { return style(s, stHead) }
func glyphOK() string       { return ok("✓") }
func glyphPending() string  { return warn("·") }
func glyphFail() string     { return bad("✗") }

// section prints a dim uppercase-ish label followed by its content.
func section(label string, lines ...string) {
	fmt.Println(dim(label))
	for _, l := range lines {
		fmt.Println("  " + l)
	}
}
