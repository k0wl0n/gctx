package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

var update = flag.Bool("update", false, "rewrite testdata/screens golden files")

// goldenSize is a terminal size the screens are committed at.
type goldenSize struct{ width, height int }

var goldenSizes = []goldenSize{{160, 40}, {120, 40}, {90, 30}, {70, 24}}

// screenScenario builds a model showing one screen over the canvas fixture.
type screenScenario struct {
	name  string
	build func(t *testing.T, size goldenSize) Model
	want  []string // text every size must show
}

func mainModel(t *testing.T, svc *fakeService, size goldenSize) Model {
	t.Helper()
	// Azure's check is still running, as on the canvas.
	return checksInFlight(t, svc, size.width, size.height,
		provider.GCP, provider.AWS, fakeClaude, fakeCodex, fakeSumoPod, fakeOpenRouter)
}

// withRealForms gives the fixture's GCP and AWS the create forms the real
// providers declare.
func withRealForms(t *testing.T, svc *fakeService) *fakeService {
	t.Helper()
	mgr, _ := managerFor(t)
	for _, real := range mgr.Providers() {
		for i, info := range svc.overview.Providers {
			if info.ID == real.ID {
				svc.overview.Providers[i].Form, svc.overview.Providers[i].Footer = real.Form, real.Footer
				svc.overview.Providers[i].InstallHint = real.InstallHint
			}
		}
	}
	return svc
}

func screenScenarios() []screenScenario {
	return []screenScenario{
		{"main", func(t *testing.T, size goldenSize) Model {
			m := mainModel(t, newMultiService(), size)
			m.selected = "prod-aws"
			return m
		}, nil},
		{"help", func(t *testing.T, size goldenSize) Model {
			svc := withTencent(newMultiService())
			svc.cliVersions = map[provider.ID]string{
				provider.GCP: "Google Cloud SDK 540.0.0", provider.AWS: "aws-cli/2.27.4 Python/3.13.4 Darwin/25.0.0",
				provider.Azure: "azure-cli                         2.75.0 *",
			}
			m := mainModel(t, svc, size)
			m.logPath = "~/.cache/gctx/tui-debug.log"
			return press(t, m, "?")
		}, []string{"navigation", "esc or ? to close"}},
		{"details", func(t *testing.T, size goldenSize) Model {
			m := mainModel(t, newMultiService(), size)
			m.selected = "prod-aws"
			return press(t, m, "d")
		}, []string{"prod-aws"}},
		{"create", func(t *testing.T, size goldenSize) Model {
			m := mainModel(t, withRealForms(t, newMultiService()), size)
			m.selected = "prod-aws"
			m = press(t, m, "c", "tab")
			m = typeText(t, m, "prod-eu-data")
			m = tabUntil(t, m, "› Profile name")
			m = typeText(t, m, "prod-eu-data")
			m = press(t, m, "tab")
			m = typeText(t, m, "https://corp.awsapps.com/start")
			m = press(t, m, "tab")
			return typeText(t, m, "eu-west-1")
		}, []string{"Profile name"}},
		{"palette", func(t *testing.T, size goldenSize) Model {
			m := mainModel(t, newMultiService(), size)
			m.selected = "cloudsuite"
			return press(t, m, ":", "l", "o", "g")
		}, []string{"log"}},
		{"delete", func(t *testing.T, size goldenSize) Model {
			svc := newMultiService()
			svc.deletePlans = map[string]manager.DeletePlan{"corp-azure": azureDeletePlan()}
			for i, a := range svc.overview.Accounts {
				if a.Name == "corp-azure" {
					svc.overview.Accounts[i].Active = true
				}
			}
			svc.overview.ActiveBySlot["azure"] = "corp-azure"
			m := mainModel(t, svc, size)
			m.selected = "corp-azure"
			return press(t, m, "ctrl+d")
		}, []string{"corp-azure"}},
		{"login", func(t *testing.T, size goldenSize) Model {
			svc := loginFixture(t)
			svc.loginFn = blockedLogin(nil,
				step(provider.LoginStepPending, stepIsolateLabel), step(provider.LoginStepPending, stepLoginLabel),
				step(provider.LoginStepStarted, stepIsolateLabel), step(provider.LoginStepDone, stepIsolateLabel),
				step(provider.LoginStepStarted, stepLoginLabel), deviceCodeEvent,
				output("$ az login --use-device-code"),
				output("To sign in, use a web browser to open the page "+testDeviceURL),
				output("and enter the code "+testDeviceCode+" to authenticate."))
			p := newLoginPump(t, svc)
			p.m = resize(t, p.m, size.width, size.height)
			p.m.now = func() time.Time { return fixtureNow }
			selectAccount(p, "corp-azure")
			p.key("l")
			return p.m
		}, []string{"corp-azure"}},
		{"firstrun", func(t *testing.T, size goldenSize) Model {
			m := firstRunModel(t, firstRunService(), size.width, size.height)
			return press(t, m, "up", "space") // AZR unchecked, as on the canvas
		}, []string{"no accounts yet", "Import 9"}},
		{"missingcli", func(t *testing.T, size goldenSize) Model {
			m := mainModel(t, withTencent(newMultiService()), size)
			return press(t, m, "5")
		}, []string{"tccli is not installed"}},
		{"offline", func(t *testing.T, size goldenSize) Model {
			svc := newMultiService()
			markOffline(svc)
			m := checksInFlight(t, svc, size.width, size.height)
			m = finishChecks(m, nil)
			m.selected = "prod-aws"
			return m
		}, []string{"network unreachable"}},
	}
}

// TestScreenGoldens renders every screen at every committed size and compares
// the plain text with testdata/screens. Regenerate with
// go test ./pkg/tui -run TestScreenGoldens -update after checking the output.
func TestScreenGoldens(t *testing.T) {
	t.Setenv("GCTX_ASCII", "")
	check := func(t *testing.T, file, got string) {
		path := filepath.Join("testdata", "screens", file)
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
	}
	for _, scenario := range screenScenarios() {
		for _, size := range goldenSizes {
			file := fmt.Sprintf("%s-%d.txt", scenario.name, size.width)
			t.Run(file, func(t *testing.T) {
				got := trimRight(screen(scenario.build(t, size))) + "\n"
				for _, want := range scenario.want {
					if !strings.Contains(got, want) {
						t.Errorf("%s lacks %q:\n%s", file, want, got)
					}
				}
				check(t, file, got)
			})
		}
	}
	t.Run("aws-120.txt", func(t *testing.T) {
		m := mainModel(t, newMultiService(), goldenSize{120, 24})
		m = press(t, m, "3")
		m.selected = "prod-aws"
		check(t, "aws-120.txt", trimRight(screen(m))+"\n")
	})
}

// TestScreensFitTheirWidth asserts no screen draws a line wider than the
// terminal or more lines than its height.
func TestScreensFitTheirWidth(t *testing.T) {
	t.Setenv("GCTX_ASCII", "")
	for _, scenario := range screenScenarios() {
		for _, size := range goldenSizes {
			t.Run(fmt.Sprintf("%s-%d", scenario.name, size.width), func(t *testing.T) {
				out := screen(scenario.build(t, size))
				rows := lines(out)
				if len(rows) > size.height {
					t.Errorf("%d lines in a %d line terminal:\n%s", len(rows), size.height, out)
				}
				for _, line := range rows {
					if w := ansi.StringWidth(line); w > size.width {
						t.Errorf("line is %d wide in a %d column terminal: %q", w, size.width, line)
					}
				}
			})
		}
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
