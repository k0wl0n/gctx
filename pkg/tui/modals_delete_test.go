package tui

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func azureDeletePlan() manager.DeletePlan {
	return manager.DeletePlan{
		Items: []provider.DeleteItem{
			{Key: provider.DeleteKeyRemoveDir, Label: "isolated az dir ~/.config/gctx/azure/corp-azure", Detail: "2.1 MB, cached tokens", Default: true},
			{Key: provider.DeleteKeyLogout, Label: "run az logout first", Detail: "revokes the refresh token server-side"},
		},
		Kept: "the Azure subscription and its resources. " + config.AppName + " never touches cloud resources.",
	}
}

// openDeleteFor selects name in the multi-provider fixture and opens the dialog.
func openDeleteFor(t *testing.T, svc *fakeService, name string, width, height int) (Model, *deleteDialog) {
	t.Helper()
	m := multiModel(t, svc, width, height)
	m.selected = name
	m = press(t, m, "ctrl+d")
	d, ok := m.modal.(*deleteDialog)
	if !ok {
		t.Fatalf("ctrl+d on %s opened %T", name, m.modal)
	}
	return m, d
}

func azureDeleteService() *fakeService {
	svc := newMultiService()
	svc.deletePlans = map[string]manager.DeletePlan{"corp-azure": azureDeletePlan()}
	return svc
}

// tabTo presses tab until the dialog focus is kind (and item, for checkboxes).
func tabTo(t *testing.T, m Model, kind deleteStopKind, item int) Model {
	t.Helper()
	for range 12 {
		d := m.modal.(*deleteDialog)
		if stop := d.current(); stop.kind == kind && (kind != stopItem || stop.item == item) {
			return m
		}
		m = press(t, m, "tab")
	}
	t.Fatalf("focus never reached %v/%d", kind, item)
	return m
}

// confirmDelete presses enter on Delete and runs the delete command only; the
// status timer that press would also run sleeps for seconds.
func confirmDelete(t *testing.T, m Model) {
	t.Helper()
	m, cmd := m.dispatch(keyMsg("enter"))
	if m.op == nil || m.op.kind != opDelete || cmd == nil {
		t.Fatal("enter on Delete did not start the delete")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected a batch")
	}
	for _, c := range batch {
		c()
	}
}

func typeInto(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = press(t, m, string(r))
	}
	return m
}

func TestDeleteDefaultFocusCancel(t *testing.T) {
	_, d := openDeleteFor(t, azureDeleteService(), "corp-azure", 120, 40)
	if d.current().kind != stopCancel {
		t.Errorf("initial focus %+v, want Cancel", d.current())
	}
}

func TestDeleteRequiresTypedName(t *testing.T) {
	svc := azureDeleteService()
	m, _ := openDeleteFor(t, svc, "corp-azure", 120, 40)

	m = tabTo(t, m, stopDelete, 0)
	m = press(t, m, "enter")
	if m.op != nil || m.modal == nil || len(svc.deleteOpts) != 0 {
		t.Fatal("Delete ran without the typed name")
	}

	m = tabTo(t, m, stopInput, 0)
	m = typeInto(t, m, "corp-az")
	if m.modal.(*deleteDialog).canDelete() {
		t.Error("a partial name enables Delete")
	}
	m = typeInto(t, m, "URE")
	if m.modal.(*deleteDialog).canDelete() {
		t.Error("a differently cased name enables Delete")
	}
	m = press(t, m, "backspace", "backspace", "backspace")
	m = typeInto(t, m, "ure")
	if !m.modal.(*deleteDialog).canDelete() {
		t.Fatal("the exact name must enable Delete")
	}

	m = tabTo(t, m, stopDelete, 0)
	confirmDelete(t, m)
	if len(svc.deleteOpts) != 1 {
		t.Fatalf("Delete calls: %v", svc.Calls())
	}
	if opts := svc.deleteOpts[0]; !opts.RemoveDir || opts.Logout || opts.GcloudConfig {
		t.Errorf("options %+v: dir removal is the default, logout is not", opts)
	}
}

func TestDeleteCheckboxTogglesOption(t *testing.T) {
	svc := azureDeleteService()
	m, _ := openDeleteFor(t, svc, "corp-azure", 120, 40)
	m = tabTo(t, m, stopItem, 2) // logout (row 0 is the locked config entry)
	m = press(t, m, "space")
	m = tabTo(t, m, stopItem, 1) // remove dir
	m = press(t, m, "space")
	m = tabTo(t, m, stopInput, 0)
	m = typeInto(t, m, "corp-azure")
	m = tabTo(t, m, stopDelete, 0)
	confirmDelete(t, m)
	if opts := svc.deleteOpts[0]; opts.RemoveDir || !opts.Logout {
		t.Errorf("options %+v, want logout on and dir removal off", opts)
	}
}

func TestDeleteEscCancels(t *testing.T) {
	svc := azureDeleteService()
	m, _ := openDeleteFor(t, svc, "corp-azure", 120, 40)
	m = tabTo(t, m, stopInput, 0)
	m = press(t, m, "esc")
	if m.modal != nil || len(svc.deleteOpts) != 0 {
		t.Error("esc must close the dialog without deleting")
	}
}

func TestDeleteActiveWarns(t *testing.T) {
	m, _ := openDeleteFor(t, azureDeleteService(), "cloudsuite", 120, 40)
	out := screen(m)
	if !strings.Contains(out, "this is the active GCP account; GCP becomes none") {
		t.Errorf("no active warning:\n%s", out)
	}
	m, _ = openDeleteFor(t, azureDeleteService(), "corp-azure", 120, 40)
	if strings.Contains(screen(m), "this is the active") {
		t.Error("warning shown for an account that is not active")
	}
}

func TestDeleteRefusedWhenLinked(t *testing.T) {
	svc := newMultiService()
	svc.deletePlans = map[string]manager.DeletePlan{"cloudsuite": {Dependants: []string{"claude-vertex", "claude-work"}}}
	m, d := openDeleteFor(t, svc, "cloudsuite", 120, 40)
	out := screen(m)
	for _, want := range []string{"cannot delete cloudsuite", "claude-vertex", "claude-work", "link"} {
		if !strings.Contains(out, want) {
			t.Errorf("refusal lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "to confirm") {
		t.Error("the confirm form must not show for a linked account")
	}
	m = press(t, m, "tab", "space", "y")
	if len(svc.deleteOpts) != 0 || m.op != nil || m.modal == nil {
		t.Fatal("a refused delete must stay a refusal")
	}
	_ = d
	m = press(t, m, "enter")
	if m.modal != nil {
		t.Error("enter must close the refusal")
	}
}

func TestDeleteDialogLayoutAt120And70(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 40}, {70, 30}} {
		m, _ := openDeleteFor(t, azureDeleteService(), "corp-azure", size.width, size.height)
		out := screen(m)
		lines := strings.Split(out, "\n")
		if len(lines) > size.height {
			t.Errorf("%dx%d: %d lines", size.width, size.height, len(lines))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > size.width {
				t.Errorf("%dx%d: line is %d wide: %q", size.width, size.height, w, line)
			}
		}
		for _, want := range []string{"Delete", "AZR", "corp-azure", "remove", config.AppName + " entry in config.json", "always",
			"isolated az dir", "run az logout first", "kept:", "to confirm", "Cancel", "esc cancel"} {
			if !strings.Contains(out, want) {
				t.Errorf("%dx%d lacks %q:\n%s", size.width, size.height, want, out)
			}
		}
	}
}
