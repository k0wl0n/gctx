package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

// modalTone picks the border colour of a modal.
type modalTone int

const (
	toneNeutral modalTone = iota
	toneDanger            // destructive confirmations
	toneWarn              // banners that need attention
)

// modalWideFrom is the width from which a modal floats over the dimmed main
// screen; narrower terminals get a full-width box.
const (
	modalWideFrom = 100
	modalMaxWidth = 84
	modalTopRows  = 4 // header, env line, tabs and rule stay visible
)

// modalSpec is the content of a modal; renderModal owns the frame, so every
// overlay shares one border, padding, title row and footer row.
type modalSpec struct {
	title  string
	hint   string   // right-aligned on the title row
	body   []string // lines no wider than modalInnerWidth
	footer string   // dim key hints under the body
	tone   modalTone
	focus  int  // body line kept visible when the body scrolls; -1 for none
	wide   bool // lists such as the palette may use up to paletteMaxWidth
	full   bool // fill the whole height, footer on the last row (full-screen panels)
}

const paletteMaxWidth = 110

// modalBoxWidth is the outer width of a modal on a screen of width cells.
func modalBoxWidth(width int, wide bool) int {
	if width >= modalWideFrom {
		if wide {
			return min(width-8, paletteMaxWidth)
		}
		return min(width-8, modalMaxWidth)
	}
	return max(width, 20)
}

// modalInnerWidth is the width left for body lines: border and padding take
// two cells a side.
func modalInnerWidth(width int, wide bool) int { return modalBoxWidth(width, wide) - 4 }

// modalMaxHeight leaves the header and tab rows visible above a floating
// modal.
func modalMaxHeight(width, height int) int {
	if width >= modalWideFrom {
		return max(height-modalTopRows-1, 5)
	}
	return max(height, 5)
}

func (t modalTone) style(st styles) lipgloss.Style {
	switch t {
	case toneDanger:
		return st.Error
	case toneWarn:
		return st.Warn
	}
	return st.Sep
}

// renderModal frames spec into box lines that fit width × height, scrolling
// the body around spec.focus when it is too tall.
func renderModal(spec modalSpec, width, height int, g glyphs, st styles) []string {
	boxWidth := modalBoxWidth(width, spec.wide)
	inner := boxWidth - 4
	border := spec.tone.style(st)
	vertical := border.Render(g.Box[5])
	row := func(content string) string {
		return vertical + " " + fitCells(content, inner, g.Ellipsis) + " " + vertical
	}

	var top []string
	if spec.title != "" {
		top = append(top, joinSides(st.Title.Render(spec.title), st.Dim.Render(spec.hint), inner, g), "")
	}
	var bottom []string
	if spec.footer != "" {
		bottom = append(bottom, "", st.Dim.Render(truncateCells(spec.footer, inner, g.Ellipsis)))
	}

	body := spec.body
	maxHeight := modalMaxHeight(width, height)
	if spec.full {
		maxHeight = height
	}
	room := maxHeight - 2 - len(top) - len(bottom)
	if room < len(body) && len(top) > 0 {
		top = top[:1] // drop the spacer rows before cutting content
		room++
	}
	if room < len(body) && len(bottom) > 0 {
		bottom = bottom[1:]
		room++
	}
	if room < len(body) {
		room = max(room, 1)
		start := 0
		if spec.focus >= 0 {
			start = max(min(spec.focus-room/2, len(body)-room), 0)
		}
		body = body[start : start+room]
	}
	for spec.full && len(body) < room {
		body = append(append([]string{}, body...), "")
	}

	out := []string{border.Render(g.Box[0] + strings.Repeat(g.Box[4], boxWidth-2) + g.Box[1])}
	for _, line := range append(append(append([]string{}, top...), body...), bottom...) {
		out = append(out, row(line))
	}
	return append(out, border.Render(g.Box[2]+strings.Repeat(g.Box[4], boxWidth-2)+g.Box[3]))
}

// overlayModal draws box centred over base, which is dimmed so the modal
// reads as the foreground while the header and tab rows stay visible.
func overlayModal(base string, box []string, width, height int, st styles) string {
	lines := strings.Split(base, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	lines = lines[:max(height, len(box))]
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = padCells(ansi.Strip(line), width)
		lines[i] = st.Dim.Render(plain[i])
	}

	boxWidth := 0
	for _, line := range box {
		boxWidth = max(boxWidth, ansi.StringWidth(line))
	}
	left := max((width-boxWidth)/2, 0)
	topRow := 0
	if width >= modalWideFrom {
		topRow = max(min(max((height-len(box))/2, modalTopRows), height-len(box)), 0)
	} else {
		topRow = max((height-len(box))/2, 0)
	}
	for i, boxLine := range box {
		y := topRow + i
		if y >= len(lines) {
			break
		}
		lines[y] = st.Dim.Render(ansi.Truncate(plain[y], left, "")) + boxLine +
			st.Dim.Render(ansi.TruncateLeft(plain[y], left+boxWidth, ""))
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}

// showModal frames spec and draws it over the dimmed main screen.
func (m Model) showModal(spec modalSpec) string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	width, height := max(m.width, minWidth), max(m.height, minHeight)
	return overlayModal(m.viewMain(), renderModal(spec, width, height, g, st), width, height, st)
}

// chip is one choice in a chip row; color is a provider badge colour.
type chip struct {
	label       string
	color       string
	unavailable bool // CLI missing: still selectable, drawn struck through
}

// chipRow draws "‹ a [b] c ›  ←/→": the selected chip is filled in its
// colour (reverse video without one), or bracketed without colour support.
func chipRow(chips []chip, selected int, g glyphs, st styles) string {
	parts := make([]string, len(chips))
	for i, c := range chips {
		switch {
		case i != selected && c.unavailable:
			parts[i] = st.Dim.Strikethrough(true).Render(c.label)
			parts[i] = " " + parts[i] + " "
		case i != selected:
			parts[i] = " " + c.label + " "
		case st.prof <= colorprofile.ASCII:
			parts[i] = "[" + c.label + "]"
		default:
			parts[i] = filledStyle(c.color, st).Render(" " + c.label + " ")
		}
	}
	left, right, arrows := "‹", "›", "←/→"
	if g.Caret == ">" {
		left, right, arrows = "<", ">", "left/right"
	}
	return st.Dim.Render(left) + strings.Join(parts, "") + st.Dim.Render(right+"  "+arrows)
}

// filledStyle is ground-coloured bold text on hex, or reverse video when hex
// has no colour in the profile.
func filledStyle(hex string, st styles) lipgloss.Style {
	if col := badgeColor(hex, st.prof); col != nil {
		return lipgloss.NewStyle().Background(col).Foreground(colorGround.in(st.prof)).Bold(true)
	}
	return st.TabActive
}

// Buttons come in three weights. renderPrimaryButton is always filled,
// renderButton (secondary) is outlined with border-coloured bars and
// renderPlainButton (tertiary) is dim text. A focused button of any weight is
// filled; without colour focus is "[*label*]" and the rest "[ label ]".
// Every state has the same width so a row never shifts.

// renderPrimaryButton is the filled call to action such as "Import 9".
func renderPrimaryButton(label string, focused, enabled bool, st styles) string {
	switch {
	case !enabled:
		return st.Dim.Render("[ " + label + " ]")
	case st.prof <= colorprofile.ASCII && focused:
		return "[*" + label + "*]"
	case st.prof <= colorprofile.ASCII:
		return "[ " + label + " ]"
	case focused:
		return st.TabActive.Underline(true).Render(" " + label + " ")
	}
	return st.TabActive.Render(" " + label + " ")
}

// renderButton is the outlined secondary button; a focused one is filled, a
// disabled one dim.
func renderButton(label string, focused, enabled bool, st styles) string {
	switch {
	case !enabled:
		return st.Dim.Render("[ " + label + " ]")
	case focused && st.prof <= colorprofile.ASCII:
		return "[*" + label + "*]"
	case focused:
		return st.TabActive.Render("  " + label + "  ")
	case st.prof <= colorprofile.ASCII:
		return "[ " + label + " ]"
	}
	bar := st.Sep.Render("│")
	if g := newGlyphs(asciiRequested()); g.Caret == ">" {
		bar = st.Sep.Render("|")
	}
	return bar + " " + label + " " + bar
}

// renderPlainButton is the tertiary button such as Cancel or Start empty.
func renderPlainButton(label string, focused bool, st styles) string {
	switch {
	case focused:
		return renderButton(label, true, true, st)
	case st.prof <= colorprofile.ASCII:
		return "  " + label + "  "
	}
	return st.Dim.Render("  " + label + "  ")
}
