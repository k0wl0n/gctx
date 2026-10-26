package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

func TestAdjacentOutlinedButtonsAreSeparated(t *testing.T) {
	st := newStylesFor(colorprofile.TrueColor)
	row := ansi.Strip(joinButtons(renderButton("a", false, true, st), renderButton("b", false, true, st)))
	if want := "│ a │  │ b │"; row != want {
		t.Errorf("button row = %q, want %q", row, want)
	}
	row = joinButtons(renderButton("a", false, true, newStylesFor(colorprofile.ASCII)), renderButton("b", false, true, newStylesFor(colorprofile.ASCII)))
	if want := "[ a ]  [ b ]"; row != want {
		t.Errorf("no-colour button row = %q, want %q", row, want)
	}
}

func TestHelpShowsProvidersWhenTheHeightAllows(t *testing.T) {
	t.Setenv("GCTX_ASCII", "")
	for _, size := range []goldenSize{{90, 56}, {70, 56}} {
		m := press(t, mainModel(t, withTencent(newMultiService()), size), "?")
		if out := screen(m); !strings.Contains(out, "providers") || !strings.Contains(out, "Codex") {
			t.Errorf("help at %dx%d hides the providers section:\n%s", size.width, size.height, out)
		}
	}
}

func TestHelpSaysWhenMoreIsBelow(t *testing.T) {
	t.Setenv("GCTX_ASCII", "")
	m := press(t, mainModel(t, withTencent(newMultiService()), goldenSize{70, 24}), "?")
	if out := screen(m); !strings.Contains(out, "more below") {
		t.Errorf("a clipped help screen must say more is below:\n%s", out)
	}
}

var touching = regexp.MustCompile(`\w │ │ \w`)

func TestButtonRowsOnEveryScreenKeepAGap(t *testing.T) {
	create := screen(typeText(t, resize(t, openAWS(t, formService(t)), 120, 40), "prod-eu-data"))
	deleteModel, _ := openDeleteFor(t, azureDeleteService(), "corp-azure", 120, 40)
	del := screen(deleteModel)
	first := screen(firstRunModel(t, firstRunService(), 120, 30))
	for name, out := range map[string]string{"create": create, "delete": del, "first-run": first} {
		if touching.MatchString(out) {
			t.Errorf("%s: outlined buttons touch:\n%s", name, out)
		}
	}
	if !strings.Contains(create, "│ Create and login │  │ Create only │") {
		t.Errorf("create buttons lack the gap:\n%s", create)
	}
}

func TestMissingCLIHintsUseDotSeparators(t *testing.T) {
	t.Setenv("GCTX_ASCII", "")
	m := press(t, mainModel(t, withTencent(newMultiService()), goldenSize{120, 30}), "5")
	if out := screen(m); !strings.Contains(out, "y copy · R rescan · tab next provider") {
		t.Errorf("missing-CLI hints lack the dot-separated line:\n%s", out)
	}
}
