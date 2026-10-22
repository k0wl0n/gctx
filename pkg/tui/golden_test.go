package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/provider"
)

var update = flag.Bool("update", false, "rewrite testdata/screens golden files")

// TestGoldenScreens renders the main screen over the canvas fixture and
// compares the plain text with testdata/screens. Regenerate with
// go test ./pkg/tui -run TestGoldenScreens -update.
func TestGoldenScreens(t *testing.T) {
	t.Setenv("GCTX_ASCII", "")
	cases := []struct {
		file          string
		width, height int
		tab           string // key pressed to pick a tab; "" stays on All
		selected      string
	}{
		{"main-160.txt", 160, 40, "", "prod-aws"},
		{"main-120.txt", 120, 40, "", "prod-aws"},
		{"aws-120.txt", 120, 24, "3", "prod-aws"},
		{"main-90.txt", 90, 30, "", "prod-aws"},
		{"main-70.txt", 70, 24, "", "prod-aws"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			// Azure's check is still running, as on the canvas.
			m := checksInFlight(t, newMultiService(), c.width, c.height,
				provider.GCP, provider.AWS, fakeClaude, fakeCodex, fakeSumoPod, fakeOpenRouter)
			if c.tab != "" {
				m = press(t, m, c.tab)
			}
			m.selected = c.selected
			got := trimRight(screen(m)) + "\n"
			path := filepath.Join("testdata", "screens", c.file)
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if got != string(want) {
				t.Errorf("%s differs from the rendered screen (run with -update after checking):\n%s", path, got)
			}
		})
	}
}

// trimRight drops trailing spaces per line so golden files stay readable.
func trimRight(s string) string {
	ls := strings.Split(s, "\n")
	for i, l := range ls {
		ls[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(ls, "\n")
}
