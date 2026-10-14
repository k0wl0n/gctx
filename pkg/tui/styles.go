package tui

import (
	"os"

	"charm.land/lipgloss/v2"
)

// glyphs are the status symbols; ASCII replaces the Unicode set.
type glyphs struct {
	Active, Yes, No, Warn, Ellipsis string
	HRule                           string // horizontal separator character
}

func newGlyphs(ascii bool) glyphs {
	if ascii {
		return glyphs{Active: "*", Yes: "+", No: "x", Warn: "!", Ellipsis: "...", HRule: "-"}
	}
	return glyphs{Active: "●", Yes: "✓", No: "✗", Warn: "⚠", Ellipsis: "…", HRule: "─"}
}

func asciiRequested() bool { return os.Getenv("GCTX_ASCII") == "1" }

// styles hold all colour. Status is always also conveyed by text.
type styles struct {
	Header, Title, Selected, ActiveName, Dim, OK, Warn, Error, Sep lipgloss.Style
}

func newStyles() styles {
	return styles{
		// Column headers: bold + subtle underline
		Header: lipgloss.NewStyle().Bold(true).Underline(true),
		// App title in header bar: bold cyan
		Title: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
		// Selected row: blue background so it's visible in both light and dark themes
		Selected: lipgloss.NewStyle().
			Background(lipgloss.Color("4")).
			Foreground(lipgloss.Color("15")).
			Bold(true),
		// Active account name: bold green
		ActiveName: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2")),
		// Dim text for hints and separators
		Dim:   lipgloss.NewStyle().Faint(true),
		OK:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		Warn:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		Error: lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		// Separator line
		Sep: lipgloss.NewStyle().Faint(true),
	}
}
