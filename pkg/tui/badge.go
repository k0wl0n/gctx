package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// badgeWidth is the cell width of every badge: three letters plus padding or
// brackets, so rows stay aligned whatever the style or colour profile.
const badgeWidth = 5

// ansi16Badge maps the spec's cloud colours to their 16-colour equivalents.
// Other colours fall back to the nearest bright ANSI colour.
var ansi16Badge = map[string]ansi.BasicColor{
	"#6ea8fe": lipgloss.BrightBlue,
	"#ffa657": lipgloss.BrightYellow,
	"#56d4dd": lipgloss.BrightCyan,
	"#c39bff": lipgloss.BrightMagenta,
}

// badgeColor resolves a badge colour for prof; nil when prof has no colour.
func badgeColor(hex string, prof colorprofile.Profile) color.Color {
	if hex == "" || prof <= colorprofile.ASCII {
		return nil
	}
	if prof != colorprofile.ANSI {
		return prof.Convert(lipgloss.Color(hex))
	}
	if basic, ok := ansi16Badge[strings.ToLower(hex)]; ok {
		return basic
	}
	if basic, ok := prof.Convert(lipgloss.Color(hex)).(ansi.BasicColor); ok {
		if basic < lipgloss.BrightBlack {
			basic += lipgloss.BrightBlack
		}
		return basic
	}
	return nil
}

// renderBadge draws a provider badge. Filled badges are a coloured block;
// outline and dashed badges cannot be drawn inline in a terminal and use
// brackets in the badge colour. Without colour every badge uses brackets so
// the letters alone carry the meaning.
func renderBadge(b provider.Badge, prof colorprofile.Profile, g glyphs) string {
	text := fitCells(b.Text, badgeWidth-2, "")
	col := badgeColor(b.Color, prof)
	if col == nil {
		return "[" + text + "]"
	}
	if b.Style == provider.BadgeFilled {
		ground := colorGround.in(prof)
		return lipgloss.NewStyle().Background(col).Foreground(ground).Bold(true).Render(" " + text + " ")
	}
	return lipgloss.NewStyle().Foreground(col).Render("[" + text + "]")
}

// tint renders text in a badge colour, for slot names and markers.
func tint(hex string, prof colorprofile.Profile, text string) string {
	col := badgeColor(hex, prof)
	if col == nil {
		return text
	}
	return lipgloss.NewStyle().Foreground(col).Render(text)
}
