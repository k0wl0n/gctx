package tui

import (
	"strings"
	"testing"
)

// TestEditNameReadOnlyWithoutRenameSupport keeps the Name field out of the
// edit form's focus order, with a reason, for a provider that cannot rename.
func TestEditNameReadOnlyWithoutRenameSupport(t *testing.T) {
	overview := multiOverview()
	for i := range overview.Providers {
		overview.Providers[i].NoRename = true
	}
	acct, _ := overview.Account("prod-aws")
	f, ok := newEditForm(overview, acct)
	if !ok {
		t.Fatal("no edit form")
	}
	for _, item := range f.items() {
		if item.kind == itemName {
			t.Fatal("name is focusable for a provider that cannot rename")
		}
	}
	m := multiModel(t, newFakeService(overview), 120, 40)
	m.modal = f
	if out := screen(m); !strings.Contains(out, "prod-aws") || !strings.Contains(out, "cannot be renamed") {
		t.Errorf("edit form:\n%s", out)
	}
}
