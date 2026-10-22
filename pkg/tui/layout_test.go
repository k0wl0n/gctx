package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// allColumns is a single-provider row with every column the layout knows.
func allColumns() []colSpec {
	return []colSpec{
		{key: colKind, header: "KIND", width: 5},
		{key: colName, header: "NAME", width: 15, minWidth: 8},
		{key: colTarget, header: "TARGET", width: 21, minWidth: 8},
		{key: colIdentity, header: "IDENTITY", width: 28, priority: dropIdentity},
		{key: colADC, header: "ADC", width: 3, priority: dropADC},
		{key: "extra:region", header: "REGION", width: 12, priority: dropExtra},
		{key: colStatus, header: "STATUS", width: 15},
		{key: colExpires, header: "EXPIRES", width: 10, priority: dropExpires},
	}
}

func headers(cols []colSpec) []string {
	var out []string
	for _, c := range cols {
		out = append(out, c.header)
	}
	return out
}

func rowWidth(cols []colSpec) int {
	total := 0
	for _, c := range cols {
		total += c.width
	}
	return total + colGap*max(len(cols)-1, 0)
}

func TestColumnDropOrder(t *testing.T) {
	cases := []struct {
		width int
		want  []string
	}{
		{160, []string{"KIND", "NAME", "TARGET", "IDENTITY", "ADC", "REGION", "STATUS", "EXPIRES"}},
		{120, []string{"KIND", "NAME", "TARGET", "IDENTITY", "ADC", "REGION", "STATUS"}},
		{90, []string{"KIND", "NAME", "TARGET", "ADC", "REGION", "STATUS"}},
		{70, []string{"KIND", "NAME", "TARGET", "STATUS"}},
	}
	for _, c := range cases {
		got := layout(c.width, allColumns())
		if !slices.Equal(headers(got), c.want) {
			t.Errorf("%d: headers %v, want %v", c.width, headers(got), c.want)
		}
		if w := rowWidth(got); w > c.width {
			t.Errorf("%d: row is %d wide", c.width, w)
		}
	}
	// Past the droppable columns TARGET shrinks; it never drops.
	got := layout(50, allColumns())
	if !slices.Equal(headers(got), []string{"KIND", "NAME", "TARGET", "STATUS"}) || got[2].width != 9 {
		t.Errorf("50: %+v", got)
	}
}

func TestNameAndStatusNeverDrop(t *testing.T) {
	cols := allColumns()
	cols[1].width = 40
	got := layout(60, cols)
	names := headers(got)
	if !slices.Contains(names, "NAME") || !slices.Contains(names, "STATUS") {
		t.Fatalf("headers %v", names)
	}
	if w := rowWidth(got); w > 60 {
		t.Errorf("row is %d wide: %+v", w, got)
	}
	// Even far below the minimum, NAME and STATUS stay.
	if names := headers(layout(20, cols)); !slices.Contains(names, "NAME") || !slices.Contains(names, "STATUS") {
		t.Errorf("headers at 20: %v", names)
	}
}

func TestTargetMiddleEllipsis(t *testing.T) {
	if got := middleEllipsis("sap-gap-cloud-asap-s", 12, "…"); got != "sap-g…sap-s" {
		t.Errorf("got %q", got)
	}
	if got := middleEllipsis("short", 12, "…"); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := middleEllipsis("sap-gap-cloud-asap-s", 12, "..."); runeLen(got) > 12 || !strings.Contains(got, "...") {
		t.Errorf("ascii got %q", got)
	}
}

var (
	badgeGCP = provider.Badge{Text: "GCP", Style: provider.BadgeFilled, Color: "#6ea8fe"}
	badgeAWS = provider.Badge{Text: "AWS", Style: provider.BadgeFilled, Color: "#ffa657"}
	badgeAZR = provider.Badge{Text: "AZR", Style: provider.BadgeFilled, Color: "#56d4dd"}
	badgeTCT = provider.Badge{Text: "TCT", Style: provider.BadgeFilled, Color: "#c39bff"}
	badgeCLD = provider.Badge{Text: "CLD", Style: provider.BadgeOutline, Color: "#e3b38c"}
	badgeORT = provider.Badge{Text: "ORT", Style: provider.BadgeDashed, Color: "#a3b1ff"}
)

func TestBadgeNoColorUsesBrackets(t *testing.T) {
	g := newGlyphs(false)
	for _, prof := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.NoTTY} {
		for _, b := range []provider.Badge{badgeGCP, badgeCLD, badgeORT} {
			got := renderBadge(b, prof, g)
			if got != "["+b.Text+"]" {
				t.Errorf("%s %s: %q", prof, b.Text, got)
			}
		}
	}
	// With colour, every style is still exactly five cells and keeps its letters.
	for _, b := range []provider.Badge{badgeGCP, badgeCLD, badgeORT} {
		got := renderBadge(b, colorprofile.TrueColor, g)
		if plain := ansi.Strip(got); ansi.StringWidth(plain) != badgeWidth || !strings.Contains(plain, b.Text) {
			t.Errorf("truecolor %s: %q", b.Text, plain)
		}
	}
}

func TestBadge16ColorMapping(t *testing.T) {
	g := newGlyphs(false)
	cases := []struct {
		badge provider.Badge
		sgr   string // background of the bright ANSI colour
	}{
		{badgeGCP, "104"}, // bright blue
		{badgeAWS, "103"}, // bright yellow
		{badgeAZR, "106"}, // bright cyan
		{badgeTCT, "105"}, // bright magenta
	}
	for _, c := range cases {
		got := renderBadge(c.badge, colorprofile.ANSI, g)
		if !strings.Contains(got, "\x1b["+c.sgr) && !strings.Contains(got, ";"+c.sgr) {
			t.Errorf("%s: want SGR %s in %q", c.badge.Text, c.sgr, got)
		}
	}
	if got := renderBadge(badgeGCP, colorprofile.TrueColor, g); !strings.Contains(got, "48;2;110;168;254") {
		t.Errorf("truecolor: %q", got)
	}
	if got := renderBadge(badgeGCP, colorprofile.ANSI256, g); !strings.Contains(got, "48;5;") {
		t.Errorf("256: %q", got)
	}
}
