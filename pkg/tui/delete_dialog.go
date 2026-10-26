package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

type deleteStopKind int

const (
	stopCancel deleteStopKind = iota
	stopItem
	stopInput
	stopDelete
)

// deleteStop is one focusable element; item indexes deleteDialog.rows.
type deleteStop struct {
	kind deleteStopKind
	item int
}

type deleteRow struct {
	item    provider.DeleteItem
	checked bool
}

// deleteDialog lists what deleting removes, asks for the account name, and
// refuses outright while other accounts link to the account (D12).
type deleteDialog struct {
	account manager.AccountStatus
	info    manager.ProviderInfo
	plan    manager.DeletePlan
	rows    []deleteRow
	input   lineInput
	stop    int // index into stops(); focus starts on Cancel
}

func newDeleteDialog(acct manager.AccountStatus, info manager.ProviderInfo, plan manager.DeletePlan) *deleteDialog {
	d := &deleteDialog{account: acct, info: info, plan: plan, input: newLineInput("")}
	d.rows = append(d.rows, deleteRow{
		item:    provider.DeleteItem{Label: config.AppName + " entry in config.json", Detail: "always", Default: true, Locked: true},
		checked: true,
	})
	for _, item := range plan.Items {
		d.rows = append(d.rows, deleteRow{item: item, checked: item.Default})
	}
	return d
}

func (d *deleteDialog) refused() bool { return len(d.plan.Dependants) > 0 }

func (d *deleteDialog) stops() []deleteStop {
	stops := []deleteStop{{kind: stopCancel}}
	if d.refused() {
		return stops
	}
	for i, row := range d.rows {
		if !row.item.Locked {
			stops = append(stops, deleteStop{kind: stopItem, item: i})
		}
	}
	return append(stops, deleteStop{kind: stopInput}, deleteStop{kind: stopDelete})
}

func (d *deleteDialog) current() deleteStop { return d.stops()[d.stop] }

func (d *deleteDialog) canDelete() bool {
	return !d.refused() && d.input.Value() == d.account.Name
}

func (d *deleteDialog) move(delta int) {
	count := len(d.stops())
	d.stop = (d.stop + delta + count) % count
}

func (d *deleteDialog) options() manager.DeleteOptions {
	var opts manager.DeleteOptions
	for _, row := range d.rows {
		opts.Set(row.item.Key, row.checked)
	}
	return opts
}

func (d *deleteDialog) update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	m.modal = d
	stop := d.current()
	switch key.String() {
	case "esc":
		m.modal = nil
		return m, nil
	case "tab", "down":
		d.move(1)
		return m, nil
	case "shift+tab", "up":
		d.move(-1)
		return m, nil
	case "left", "right":
		if stop.kind == stopDelete || stop.kind == stopCancel {
			d.toggleButtons()
			return m, nil
		}
	case "space":
		if stop.kind == stopItem {
			d.rows[stop.item].checked = !d.rows[stop.item].checked
			return m, nil
		}
	case "enter":
		return d.activate(m, stop)
	}
	if stop.kind == stopInput {
		d.input.update(key)
	}
	return m, nil
}

func (d *deleteDialog) toggleButtons() {
	if d.current().kind == stopDelete {
		d.stop = 0
		return
	}
	d.stop = len(d.stops()) - 1
}

func (d *deleteDialog) activate(m Model, stop deleteStop) (Model, tea.Cmd) {
	switch stop.kind {
	case stopCancel:
		m.modal = nil
	case stopItem:
		d.rows[stop.item].checked = !d.rows[stop.item].checked
	case stopInput:
		if d.canDelete() {
			d.stop = len(d.stops()) - 1
		}
	case stopDelete:
		if !d.canDelete() {
			return m, nil
		}
		m.modal = nil
		if m.busy() {
			return m, m.setStatus("Busy: "+m.op.desc, levelWarning)
		}
		m.seq++
		seq := m.seq
		name, opts := d.account.Name, d.options()
		m.op = &operation{kind: opDelete, account: name, desc: "deleting", seq: seq, started: m.now()}
		cmd := func() tea.Msg {
			result, err := m.svc.Delete(context.Background(), name, opts)
			return opDoneMsg{seq: seq, kind: opDelete, account: name, result: result, err: err}
		}
		return m, tea.Batch(func() tea.Msg { return m.spin.Tick() }, cmd)
	}
	return m, nil
}

func (d *deleteDialog) view(m Model) string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	width := modalInnerWidth(max(m.width, minWidth), false)
	stop := d.current()

	var lines []string
	focus := -1
	add := func(text string) { lines = append(lines, ansi.Truncate(text, width, "…")) }
	title := "Delete " + renderBadge(d.info.Badge, m.profile, g) + " " + d.account.Name
	wrapped := func(indent int, first, text string, style lipgloss.Style) {
		pad := strings.Repeat(" ", indent)
		for n, line := range wrapText(text, width-indent-ansi.StringWidth(first)) {
			prefix := pad + strings.Repeat(" ", ansi.StringWidth(first))
			if n == 0 {
				prefix = pad + first
			}
			add(prefix + style.Render(line))
		}
	}
	plain := lipgloss.NewStyle()

	if d.refused() {
		wrapped(0, g.Warn+" ", "cannot delete "+d.account.Name+": other accounts link to it", st.Warn)
		for _, name := range d.plan.Dependants {
			add("  " + g.Linked + " " + name)
		}
		add("")
		add("delete or relink them first")
		add("")
		add(renderButton("Close", true, true, st))
		return m.showModal(modalSpec{title: title, body: lines, footer: "esc close", tone: toneDanger, focus: -1})
	}

	if d.account.Active {
		wrapped(0, g.Warn+" ", "this is the active "+d.info.Label+" account; "+d.slotName()+" becomes none", st.Warn)
		add("")
	}
	add(st.Dim.Render("remove"))
	for i, row := range d.rows {
		box := "[ ]"
		if row.checked {
			box = "[x]"
		}
		caret := "  "
		if stop.kind == stopItem && stop.item == i {
			caret = g.Caret + " "
			focus = len(lines)
		}
		text := row.item.Label
		if row.item.Detail != "" {
			text += " " + g.Dot + " " + row.item.Detail
		}
		style := plain
		if row.item.Locked {
			style = st.Dim
		}
		wrapped(0, caret+box+" ", text, style)
	}
	add("")
	if d.plan.Kept != "" {
		wrapped(0, "kept: ", d.plan.Kept, st.Dim)
		add("")
	}
	add("type " + st.Bold.Render(d.account.Name) + " to confirm")
	if stop.kind == stopInput {
		focus = len(lines)
	}
	add("  " + d.input.View(stop.kind == stopInput))
	inputLine := st.Sep
	if stop.kind == stopInput {
		inputLine = lipgloss.NewStyle()
	}
	add("  " + inputLine.Render(strings.Repeat(g.HRule, max(width-2, 1))))
	add("")

	deleteButton := renderButton("Delete", stop.kind == stopDelete, d.canDelete(), st)
	if d.canDelete() && stop.kind == stopDelete && st.prof > colorprofile.ASCII {
		deleteButton = filledStyle(colorError.hex, st).Render("  Delete  ")
	}
	if stop.kind == stopDelete || stop.kind == stopCancel {
		focus = len(lines)
	}
	add(joinButtons(deleteButton, renderButton("Cancel", stop.kind == stopCancel, true, st)))
	footer := "esc cancel " + g.Dot + " default focus on Cancel " + g.Dot + " tab moves " + g.Dot + " space toggles"
	return m.showModal(modalSpec{title: title, body: lines, footer: footer, tone: toneDanger, focus: focus})
}

// slotName is the short name shown where the account's slot is meant.
func (d *deleteDialog) slotName() string {
	if d.info.Badge.Text != "" {
		return d.info.Badge.Text
	}
	return strings.ToUpper(d.info.Slot)
}
