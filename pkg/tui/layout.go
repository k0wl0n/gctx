package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// tier is the width class of the terminal; it decides the screen structure,
// while layout decides which columns fit.
type tier int

const (
	tierNarrow   tier = iota // 60-99
	tierStandard             // 100-149
	tierWide                 // ≥150
)

func tierFor(width int) tier {
	switch {
	case width >= 150:
		return tierWide
	case width >= 100:
		return tierStandard
	}
	return tierNarrow
}

// Column keys. Provider columns use "extra:" + Column.Key.
const (
	colKind     = "kind"
	colName     = "name"
	colTarget   = "target"
	colIdentity = "identity"
	colADC      = "adc"
	colStatus   = "status"
	colExpires  = "expires"
	extraPrefix = "extra:"
)

// Drop priorities: the lowest positive priority drops first; 0 never drops.
const (
	dropNever = iota
	dropExpires
	dropIdentity
	dropADC
	dropExtra
)

const colGap = 2

// colSpec is one table column. A column with minWidth > 0 shrinks (TARGET
// with a middle ellipsis, then NAME) once every droppable column is gone.
type colSpec struct {
	key      string
	header   string
	width    int
	minWidth int
	priority int
}

// layout drops columns by priority until the row fits width, then shrinks
// the shrinkable columns in order (TARGET before NAME). Columns joined by
// colGap; the marker column is the caller's.
func layout(width int, cols []colSpec) []colSpec {
	kept := append([]colSpec(nil), cols...)
	for rowWidthOf(kept) > width {
		drop := -1
		for i, c := range kept {
			if c.priority != dropNever && (drop < 0 || c.priority < kept[drop].priority) {
				drop = i
			}
		}
		if drop < 0 {
			break
		}
		kept = append(kept[:drop], kept[drop+1:]...)
	}
	for _, key := range []string{colTarget, colName} {
		for i := range kept {
			over := rowWidthOf(kept) - width
			if over <= 0 {
				return kept
			}
			if kept[i].key == key && kept[i].minWidth > 0 {
				kept[i].width = max(kept[i].minWidth, kept[i].width-over)
			}
		}
	}
	return kept
}

func rowWidthOf(cols []colSpec) int {
	total := colGap * max(len(cols)-1, 0)
	for _, c := range cols {
		total += c.width
	}
	return total
}

func hasColumn(cols []colSpec, key string) bool {
	for _, c := range cols {
		if c.key == key {
			return true
		}
	}
	return false
}

// middleEllipsis keeps the start and end of s, which tell targets such as
// project ids apart better than the start alone.
func middleEllipsis(s string, n int, ellipsis string) string {
	if runeLen(s) <= n {
		return s
	}
	keep := (n - runeLen(ellipsis)) / 2
	if keep <= 0 {
		return truncateCells(s, n, ellipsis)
	}
	runes := []rune(s)
	return string(runes[:keep]) + ellipsis + string(runes[len(runes)-keep:])
}

// truncateCells cuts s, which may hold ANSI styling, to n cells.
func truncateCells(s string, n int, ellipsis string) string {
	if n <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= n {
		return s
	}
	if n <= ansi.StringWidth(ellipsis) {
		return ansi.Truncate(s, n, "")
	}
	return ansi.Truncate(s, n, ellipsis)
}

// padCells pads s, which may hold ANSI styling, to n cells.
func padCells(s string, n int) string {
	if w := ansi.StringWidth(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

// fitCells pads or cuts s to exactly n cells.
func fitCells(s string, n int, ellipsis string) string {
	return padCells(truncateCells(s, n, ellipsis), n)
}
