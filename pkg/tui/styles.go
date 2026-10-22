package tui

import (
	"image/color"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// glyphs are the status symbols; ASCII replaces the Unicode set.
type glyphs struct {
	Active, Yes, No, Warn, Ellipsis string
	HRule                           string // horizontal separator character
	Unverified, Missing, Linked     string
	Bar, VSep, Dot, Enter           string // cursor bar, column separator, middle dot, enter key
	RadioOn, RadioOff, Caret        string // form radio choices and the focused-row marker
	Spinner                         []string
}

func newGlyphs(ascii bool) glyphs {
	if ascii {
		return glyphs{
			Active: "*", Yes: "+", No: "x", Warn: "!", Ellipsis: "...", HRule: "-",
			Unverified: "?", Missing: "-", Linked: "->",
			Bar: "|", VSep: "|", Dot: ".", Enter: "enter",
			RadioOn: "(*)", RadioOff: "( )", Caret: ">",
			Spinner: []string{"|", "/", "-", "\\"},
		}
	}
	return glyphs{
		Active: "●", Yes: "✓", No: "✗", Warn: "⚠", Ellipsis: "…", HRule: "─",
		Unverified: "?", Missing: "○", Linked: "↳",
		Bar: "▌", VSep: "│", Dot: "·", Enter: "↵",
		RadioOn: "◉", RadioOff: "○", Caret: "›",
		Spinner: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

func asciiRequested() bool { return os.Getenv("GCTX_ASCII") == "1" }

// Palette from the spec's visual language. Each colour also has a 16-colour
// fallback; 256-colour terminals get the nearest index.
var (
	colorReady   = paletteColor{"#3fb950", lipgloss.Green}
	colorWarning = paletteColor{"#d29922", lipgloss.Yellow}
	colorError   = paletteColor{"#f85149", lipgloss.Red}
	colorDim     = paletteColor{"#8b949e", lipgloss.BrightBlack}
	colorCursor  = paletteColor{"#1f2a3a", nil} // 16-colour: reverse video
	colorGround  = paletteColor{"#0d1117", lipgloss.Black}
)

type paletteColor struct {
	hex    string
	ansi16 color.Color
}

// in returns the colour for a profile, or nil when the profile has no colour.
func (c paletteColor) in(prof colorprofile.Profile) color.Color {
	switch {
	case prof <= colorprofile.ASCII:
		return nil
	case prof == colorprofile.ANSI:
		return c.ansi16
	}
	return prof.Convert(lipgloss.Color(c.hex))
}

// styles hold all colour. Status is always also conveyed by text.
type styles struct {
	Header, Title, Selected, ActiveName, Dim, OK, Warn, Error, Sep lipgloss.Style
	Bold, TabActive                                                lipgloss.Style
	prof                                                           colorprofile.Profile
}

func newStyles() styles { return newStylesFor(colorprofile.TrueColor) }

func newStylesFor(prof colorprofile.Profile) styles {
	fg := func(c paletteColor) lipgloss.Style {
		if col := c.in(prof); col != nil {
			return lipgloss.NewStyle().Foreground(col)
		}
		return lipgloss.NewStyle()
	}
	selected := lipgloss.NewStyle()
	if cursor := colorCursor.in(prof); cursor != nil {
		selected = selected.Background(cursor)
	} else if prof == colorprofile.ANSI {
		selected = selected.Reverse(true)
	}
	tab := lipgloss.NewStyle().Reverse(true).Bold(true)
	return styles{
		Header:     fg(colorDim),
		Title:      lipgloss.NewStyle().Bold(true),
		Selected:   selected,
		ActiveName: lipgloss.NewStyle().Bold(true),
		Dim:        fg(colorDim),
		OK:         fg(colorReady),
		Warn:       fg(colorWarning),
		Error:      fg(colorError),
		Sep:        fg(colorDim),
		Bold:       lipgloss.NewStyle().Bold(true),
		TabActive:  tab,
		prof:       prof,
	}
}
