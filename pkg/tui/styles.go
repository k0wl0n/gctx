package tui

import (
	"os"

	"charm.land/lipgloss/v2"
)

// glyphs are the status symbols; ASCII replaces the Unicode set.
type glyphs struct {
	Active, Yes, No, Warn, Ellipsis string
}

func newGlyphs(ascii bool) glyphs {
	if ascii {
		return glyphs{Active: "*", Yes: "+", No: "x", Warn: "!", Ellipsis: "..."}
	}
	return glyphs{Active: "●", Yes: "✓", No: "✗", Warn: "⚠", Ellipsis: "…"}
}

func asciiRequested() bool { return os.Getenv("GCTX_ASCII") == "1" }

// styles hold all colour. Status is always also conveyed by text.
type styles struct {
	Header, Title, Selected, Dim, OK, Warn, Error lipgloss.Style
}

func newStyles() styles {
	return styles{
		Header:   lipgloss.NewStyle().Bold(true),
		Title:    lipgloss.NewStyle().Bold(true),
		Selected: lipgloss.NewStyle().Reverse(true),
		Dim:      lipgloss.NewStyle().Faint(true),
		OK:       lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		Warn:     lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		Error:    lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	}
}
