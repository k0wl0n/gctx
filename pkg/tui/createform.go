package tui

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/secretref"
)

// kindLabels names the categories in the Kind row, in display order.
var kindLabels = []struct {
	category provider.Category
	label    string
}{
	{provider.CategoryCloud, "Cloud"},
	{provider.CategoryAITool, "AI tool"},
	{provider.CategoryAIGateway, "AI gateway"},
}

const (
	rawKeyMessage = "store a reference, not the key"
	uniqueMessage = "unique across all accounts"
	formIndent    = 2
)

type formItemKind int

const (
	itemKind formItemKind = iota
	itemProvider
	itemName
	itemField
	itemLogin
	itemSubmit
	itemTest
	itemCancel
)

type formItem struct {
	kind  formItemKind
	field int // index into accountForm.fields for itemField
}

// formField is one provider form field with its current value.
type formField struct {
	spec  provider.FormField
	input lineInput // text fields
	value string    // option fields: the chosen option
}

func (f formField) isOption() bool { return len(f.spec.Options) > 0 }

func (f formField) current() string {
	if f.isOption() {
		return f.value
	}
	return strings.TrimSpace(f.input.Value())
}

// accountForm creates an account, or edits one when editing is set. It is
// driven entirely by provider metadata.
type accountForm struct {
	overview manager.Overview
	editing  *manager.AccountStatus

	kinds     []provider.Category
	kind      int
	providers []manager.ProviderInfo // providers of the selected kind
	provider  int

	name      lineInput
	fields    []formField
	pos       int // index into items()
	attempted bool
	test      connectionTest
}

// connectionTest is the state of the "Test again" check.
type connectionTest struct {
	seq     int
	running bool
	done    bool
	detail  string
	err     error
}

// connTestedMsg carries the result of a connection check back to the form.
type connTestedMsg struct {
	seq    int
	result provider.ConnectionResult
	err    error
}

func newCreateForm(overview manager.Overview, preferred provider.ID) *accountForm {
	f := &accountForm{overview: overview, name: newLineInput("")}
	for _, entry := range kindLabels {
		if len(providersOf(overview, entry.category)) > 0 {
			f.kinds = append(f.kinds, entry.category)
		}
	}
	for i, kind := range f.kinds {
		for j, info := range providersOf(overview, kind) {
			if info.ID == preferred {
				f.kind, f.provider = i, j
			}
		}
	}
	f.selectKind(f.kind, f.provider)
	f.pos = 1 // Provider
	return f
}

func newEditForm(overview manager.Overview, acct manager.AccountStatus) (*accountForm, bool) {
	var info manager.ProviderInfo
	found := false
	for _, candidate := range overview.Providers {
		if candidate.ID == acct.Provider {
			info, found = candidate, true
		}
	}
	if !found {
		return nil, false
	}
	f := &accountForm{overview: overview, editing: &acct, name: newLineInput("")}
	f.providers = []manager.ProviderInfo{info}
	f.name.SetValue(acct.Name)
	f.rebuildFields()
	for i := range f.fields {
		field := &f.fields[i]
		stored := acct.Settings[field.spec.Key]
		if field.spec.Key == provider.FormTargetKey {
			stored = acct.ProjectID
		}
		if field.isOption() {
			if stored != "" {
				field.value = stored
			}
		} else {
			field.input.SetValue(stored)
		}
	}
	return f, true
}

func providersOf(overview manager.Overview, category provider.Category) []manager.ProviderInfo {
	var out []manager.ProviderInfo
	for _, info := range overview.Providers {
		if info.Category == category {
			out = append(out, info)
		}
	}
	return out
}

func (f *accountForm) selectKind(kind, providerIndex int) {
	if len(f.kinds) == 0 {
		return
	}
	f.kind = kind
	f.providers = providersOf(f.overview, f.kinds[kind])
	f.provider = min(providerIndex, len(f.providers)-1)
	f.test = connectionTest{seq: f.test.seq}
	f.rebuildFields()
}

func (f *accountForm) info() (manager.ProviderInfo, bool) {
	if f.provider < len(f.providers) {
		return f.providers[f.provider], true
	}
	return manager.ProviderInfo{}, false
}

// rebuildFields resets the fields to the selected provider's defaults.
func (f *accountForm) rebuildFields() {
	f.fields = nil
	info, ok := f.info()
	if !ok {
		return
	}
	for _, spec := range info.Form {
		field := formField{spec: spec, input: newLineInput("")}
		if field.isOption() {
			field.value = spec.Default
			if field.value == "" {
				field.value = spec.Options[0]
			}
		} else {
			field.input.SetValue(spec.Default)
		}
		f.fields = append(f.fields, field)
	}
}

// values holds every field value, hidden ones included, plus the name.
func (f *accountForm) values() map[string]string {
	values := map[string]string{provider.FormNameKey: strings.TrimSpace(f.name.Value())}
	for _, field := range f.fields {
		values[field.spec.Key] = field.current()
	}
	return values
}

func (f *accountForm) fieldVisible(field formField, values map[string]string) bool {
	return field.spec.Visible == nil || field.spec.Visible(values)
}

// nameLocked reports an edit of an account whose provider cannot rename.
func (f *accountForm) nameLocked() bool {
	info, _ := f.info()
	return f.editing != nil && info.NoRename
}

func (f *accountForm) readOnly(field formField) bool {
	return f.editing != nil && !field.spec.Editable
}

func (f *accountForm) createDisabled() bool {
	info, ok := f.info()
	return f.editing == nil && (!ok || !info.CLI.Found)
}

// items lists the focusable rows in tab order.
func (f *accountForm) items() []formItem {
	var items []formItem
	if f.editing == nil {
		items = append(items, formItem{kind: itemKind}, formItem{kind: itemProvider})
	}
	if !f.nameLocked() {
		items = append(items, formItem{kind: itemName})
	}
	values := f.values()
	for i, field := range f.fields {
		if f.fieldVisible(field, values) && !f.readOnly(field) {
			items = append(items, formItem{kind: itemField, field: i})
		}
	}
	info, _ := f.info()
	if !f.createDisabled() {
		if f.editing == nil && info.CanLogin {
			items = append(items, formItem{kind: itemLogin})
		}
		items = append(items, formItem{kind: itemSubmit})
		if info.CanTest {
			items = append(items, formItem{kind: itemTest})
		}
	}
	return append(items, formItem{kind: itemCancel})
}

func (f *accountForm) focused() formItem {
	items := f.items()
	f.pos = max(min(f.pos, len(items)-1), 0)
	return items[f.pos]
}

func (f *accountForm) move(delta int) {
	count := len(f.items())
	f.pos = (f.pos + delta + count) % count
}

// formCheck is the live validation result.
type formCheck struct {
	name   string // error text, "" when valid
	fields map[string]string
}

func (c formCheck) blocked() bool { return c.name != "" || len(c.fields) > 0 }

func (f *accountForm) check() formCheck {
	check := formCheck{fields: map[string]string{}}
	name := strings.TrimSpace(f.name.Value())
	switch {
	case name == "":
		if f.attempted {
			check.name = "enter a name"
		}
	default:
		if err := manager.ValidateAccountName(name); err != nil {
			check.name = err.Error()
		} else if owner, taken := f.nameOwner(name); taken {
			check.name = "already used by " + owner
		}
	}

	values := f.values()
	for _, field := range f.fields {
		if !f.fieldVisible(field, values) || f.readOnly(field) {
			continue
		}
		value := field.current()
		switch {
		case field.spec.Secret && secretref.LooksLikeKey(value):
			check.fields[field.spec.Key] = rawKeyMessage
		case field.spec.Validate != nil && (value != "" || f.attempted):
			if err := field.spec.Validate(value); err != nil {
				check.fields[field.spec.Key] = err.Error()
			}
		}
	}
	return check
}

// nameOwner reports which provider's account already uses name.
func (f *accountForm) nameOwner(name string) (string, bool) {
	for _, acct := range f.overview.Accounts {
		if acct.Name != name || (f.editing != nil && acct.Name == f.editing.Name) {
			continue
		}
		label := string(acct.Provider)
		for _, info := range f.overview.Providers {
			if info.ID == acct.Provider {
				label = info.Label
			}
		}
		return label + " account " + name, true
	}
	return "", false
}

func (f *accountForm) update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	if done, ok := msg.(connTestedMsg); ok {
		if done.seq == f.test.seq {
			f.test = connectionTest{seq: done.seq, done: true, detail: done.result.Detail, err: done.err}
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	m.modal = f
	item := f.focused()
	switch k := key.String(); k {
	case "esc":
		m.modal = nil
		return m, nil
	case "tab", "down":
		f.move(1)
		return m, nil
	case "shift+tab", "up":
		f.move(-1)
		return m, nil
	case "enter":
		switch item.kind {
		case itemLogin:
			return f.submit(m, true)
		case itemSubmit:
			return f.submit(m, false)
		case itemTest:
			return f.startTest(m)
		case itemCancel:
			m.modal = nil
			return m, nil
		}
		f.move(1)
		return m, nil
	case "left", "right":
		if f.step(item, k == "right") {
			return m, nil
		}
	case "space":
		if item.kind == itemField && f.fields[item.field].isOption() {
			f.cycle(item.field)
			return m, nil
		}
	}

	switch item.kind {
	case itemName:
		f.name.update(key)
	case itemField:
		if field := &f.fields[item.field]; !field.isOption() {
			field.input.update(key)
		}
	}
	return m, nil
}

// step handles left and right on rows that choose rather than edit.
func (f *accountForm) step(item formItem, forward bool) bool {
	delta := -1
	if forward {
		delta = 1
	}
	switch item.kind {
	case itemKind:
		if next := f.kind + delta; next >= 0 && next < len(f.kinds) {
			f.selectKind(next, 0)
		}
		return true
	case itemProvider:
		if next := f.provider + delta; next >= 0 && next < len(f.providers) {
			f.selectKind(f.kind, next)
		}
		return true
	case itemField:
		field := &f.fields[item.field]
		if !field.isOption() {
			return false
		}
		for i, option := range field.spec.Options {
			if option == field.value {
				field.value = field.spec.Options[max(min(i+delta, len(field.spec.Options)-1), 0)]
				break
			}
		}
		return true
	}
	return false
}

func (f *accountForm) cycle(index int) {
	field := &f.fields[index]
	for i, option := range field.spec.Options {
		if option == field.value {
			field.value = field.spec.Options[(i+1)%len(field.spec.Options)]
			return
		}
	}
}

func (f *accountForm) submit(m Model, login bool) (Model, tea.Cmd) {
	f.attempted = true
	if check := f.check(); check.blocked() {
		f.focusFirstError(check)
		return m, nil
	}
	if m.busy() {
		return m, m.setStatus("Busy: "+m.op.desc, levelWarning)
	}
	m.modal = nil
	m.seq++
	seq := m.seq
	svc := m.svc
	name := strings.TrimSpace(f.name.Value())

	if f.editing != nil {
		original := f.editing.Name
		request := manager.UpdateRequest{Settings: f.changedSettings()}
		if name != original {
			request.NewName = name
		}
		m.op = &operation{kind: opUpdate, account: name, desc: "saving", seq: seq, started: m.now()}
		cmd := func() tea.Msg {
			err := svc.UpdateAccount(context.Background(), original, request)
			return opDoneMsg{seq: seq, kind: opUpdate, account: name, err: err}
		}
		return m, tea.Batch(func() tea.Msg { return m.spin.Tick() }, cmd)
	}

	request := f.createRequest(name)
	m.op = &operation{kind: opCreate, account: name, desc: "creating", seq: seq, started: m.now(), loginAfter: login}
	cmd := func() tea.Msg {
		result, err := svc.CreateAccount(context.Background(), request)
		return opDoneMsg{seq: seq, kind: opCreate, account: name, result: result, err: err}
	}
	return m, tea.Batch(func() tea.Msg { return m.spin.Tick() }, cmd)
}

// startTest re-runs the provider's connection check for the values on the form.
func (f *accountForm) startTest(m Model) (Model, tea.Cmd) {
	if f.test.running {
		return m, nil
	}
	f.test = connectionTest{seq: f.test.seq + 1, running: true}
	seq := f.test.seq
	svc := m.svc
	request := f.createRequest(strings.TrimSpace(f.name.Value()))
	return m, func() tea.Msg {
		result, err := svc.TestConnection(context.Background(), request)
		return connTestedMsg{seq: seq, result: result, err: err}
	}
}

func (f *accountForm) focusFirstError(check formCheck) {
	for pos, item := range f.items() {
		switch {
		case item.kind == itemName && check.name != "":
			f.pos = pos
			return
		case item.kind == itemField && check.fields[f.fields[item.field].spec.Key] != "":
			f.pos = pos
			return
		}
	}
}

// createRequest carries the visible fields; the target field maps to Target.
func (f *accountForm) createRequest(name string) manager.CreateRequest {
	info, _ := f.info()
	request := manager.CreateRequest{Provider: info.ID, Name: name, Fields: map[string]string{}}
	values := f.values()
	for _, field := range f.fields {
		value := field.current()
		switch {
		case !f.fieldVisible(field, values):
		case field.spec.Key == provider.FormTargetKey:
			request.Target = value
		case value != "":
			request.Fields[field.spec.Key] = value
		}
	}
	return request
}

// changedSettings is the editable fields whose value differs from the stored one.
func (f *accountForm) changedSettings() map[string]string {
	changed := map[string]string{}
	for _, field := range f.fields {
		if field.spec.Editable && field.current() != f.editing.Settings[field.spec.Key] {
			changed[field.spec.Key] = field.current()
		}
	}
	return changed
}

// ─── View ────────────────────────────────────────────────────────────────────

func (f *accountForm) view(m Model) string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	width := max(m.width, minWidth)
	inner := modalInnerWidth(width, false)
	check := f.check()
	item := f.focused()
	values := f.values()
	info, hasProvider := f.info()

	labelWidth := len("Provider")
	for _, field := range f.fields {
		labelWidth = max(labelWidth, ansi.StringWidth(field.spec.Label))
	}
	labelWidth = min(labelWidth, max(inner/4, 8), 18)
	valueIndent := 2 + labelWidth + 1
	valueWidth := max(inner-valueIndent, 10)

	var body []string
	focusLine := -1
	compact := false
	label := func(text string, focused, dim bool) string {
		caret := "  "
		if focused {
			caret = g.Caret + " "
		}
		text = fitCells(text, labelWidth, "")
		switch {
		case dim:
			text = st.Dim.Render(text)
		case focused:
			text = st.Bold.Render(text)
		}
		return caret + text + " "
	}
	row := func(labelText, value string, focused, dim bool) {
		if focused {
			focusLine = len(body)
		}
		body = append(body, label(labelText, focused, dim)+value)
	}
	under := func(text string, style lipgloss.Style) {
		for _, wrapped := range wrapText(text, valueWidth) {
			body = append(body, strings.Repeat(" ", valueIndent)+style.Render(wrapped))
		}
	}
	// input draws a text field: the value, then an underline the width of
	// the field so an empty field still shows where to type.
	input := func(labelText, value string, focused, dim bool, invalid bool) {
		value = truncateCells(value, valueWidth, g.Ellipsis)
		if compact {
			// No room for a rule under each field: fill the field instead.
			fill := st.Sep.Render(strings.Repeat("_", max(valueWidth-ansi.StringWidth(value), 0)))
			row(labelText, value+fill, focused, dim)
			return
		}
		row(labelText, value, focused, dim)
		lineStyle := st.Sep
		switch {
		case invalid:
			lineStyle = st.Error
		case focused:
			lineStyle = lipgloss.NewStyle()
		}
		body = append(body, strings.Repeat(" ", valueIndent)+lineStyle.Render(strings.Repeat(g.HRule, valueWidth)))
	}

	// Roomy first, with a rule under each input like the design; compact
	// when that does not fit the terminal.
	for _, compact = range []bool{false, true} {
		body, focusLine = nil, -1
		if !hasProvider {
			body = append(body, st.Dim.Render("No providers are available."), "")
		}
		if f.editing != nil {
			row("Provider", info.Label, false, true)
		} else if hasProvider {
			var kindChips []chip
			for _, kind := range f.kinds {
				kindChips = append(kindChips, chip{label: kindLabel(kind)})
			}
			row("Kind", chipRow(kindChips, f.kind, g, st), item.kind == itemKind, false)
			var providerChips []chip
			for _, candidate := range f.providers {
				providerChips = append(providerChips, chip{label: candidate.Label, color: candidate.Badge.Color, unavailable: !candidate.CLI.Found})
			}
			row(providerRowLabel(f.kinds[f.kind]), chipRow(providerChips, f.provider, g, st), item.kind == itemProvider, false)
			body = append(body, "")
		}

		if hasProvider && f.nameLocked() {
			row("Name", f.editing.Name, false, true)
			under(info.Label+" accounts cannot be renamed", st.Dim)
		} else if hasProvider {
			focusedName := item.kind == itemName
			input("Name", f.name.View(focusedName), focusedName, false, check.name != "")
			switch {
			case check.name != "":
				under(g.No+" "+check.name, st.Error)
			case strings.TrimSpace(f.name.Value()) != "":
				under(g.Yes+" "+uniqueMessage, st.OK)
			}
		}

		for i, field := range f.fields {
			if !f.fieldVisible(field, values) {
				continue
			}
			focused := item.kind == itemField && item.field == i
			readOnly := f.readOnly(field)
			if readOnly && field.current() == "" {
				continue
			}
			help := field.spec.Help
			if readOnly {
				help = ""
			}
			errorText := check.fields[field.spec.Key]
			if field.isOption() {
				row(field.spec.Label, radioRow(field, g, st, readOnly), focused, readOnly)
				if errorText != "" {
					under(g.No+" "+errorText, st.Error)
				} else if help != "" {
					under(help, st.Dim)
				}
				continue
			}
			var value string
			switch {
			case field.spec.Secret && secretref.LooksLikeKey(field.current()):
				value = f.maskedInput(field, focused)
			case readOnly:
				value = st.Dim.Render(field.input.View(false))
			default:
				value = field.input.View(focused)
			}
			inline := help != "" && ansi.StringWidth(value)+3+ansi.StringWidth(help) <= valueWidth
			if inline && field.current() == "" && !focused {
				value = st.Dim.Render(help)
			} else if inline {
				value += st.Dim.Render(" " + g.Dot + " " + help)
			}
			if readOnly {
				row(field.spec.Label, value, false, true)
			} else {
				input(field.spec.Label, value, focused, false, errorText != "")
			}
			if errorText != "" {
				under(g.No+" "+errorText, st.Error)
			} else if help != "" && !inline {
				under(help, st.Dim)
			}
		}

		if hasProvider {
			if f.createDisabled() {
				body = append(body, "", st.Error.Render(truncateCells(g.No+" "+cliName(info)+" not found: create is disabled", inner, g.Ellipsis)))
				for _, wrapped := range wrapText(installText(info), inner-2) {
					body = append(body, "  "+st.Dim.Render(wrapped))
				}
			}
			if info.Footer != nil && f.editing == nil {
				writes, never := info.Footer(values)
				body = append(body, hRule(g, st, inner))
				for _, footer := range []struct{ label, text string }{{"writes", writes}, {"never ", never}} {
					for n, wrapped := range wrapText(footer.text, inner-7) {
						prefix := strings.Repeat(" ", 6)
						if n == 0 {
							prefix = footer.label
						}
						body = append(body, st.Dim.Render(prefix+" "+wrapped))
					}
				}
			}
		}
		if len(body)+7 <= modalMaxHeight(width, m.height) {
			break
		}
	}
	if compact {
		if focusLine >= 0 {
			focusLine -= focusLine - len(slices.DeleteFunc(slices.Clone(body[:focusLine]), func(line string) bool { return line == "" }))
		}
		body = slices.DeleteFunc(body, func(line string) bool { return line == "" })
	}
	body = append(body, "")
	if line := f.testLine(g, st, inner); line != "" {
		body = append(body, line, "")
	}
	buttons := f.buttons(item, info, g, st)
	if item.kind >= itemLogin {
		focusLine = len(body)
	}
	hint := "tab next  ⇧tab back  esc cancel"
	if g.Caret == ">" {
		hint = "tab next  shift+tab back  esc cancel"
	}
	if ansi.StringWidth(buttons)+2+ansi.StringWidth(hint) <= inner {
		body = append(body, joinSides(buttons, st.Dim.Render(hint), inner, g))
		hint = ""
	} else {
		body = append(body, buttons)
	}

	return m.showModal(modalSpec{title: f.title(), hint: f.contextHint(info), body: body, footer: hint, focus: focusLine})
}

func (f *accountForm) title() string {
	if f.editing != nil {
		return "Edit account"
	}
	return "Create account"
}

// contextHint is the right-aligned hint on the title row.
func (f *accountForm) contextHint(info manager.ProviderInfo) string {
	switch {
	case f.editing != nil:
		return info.Label + " " + f.editing.Name
	case info.CanLogin:
		return "step 1 of 2 · then login"
	case info.Label != "":
		return info.Label + " · no login step"
	}
	return ""
}

// providerRowLabel names the chip row of providers after the kind.
func providerRowLabel(category provider.Category) string {
	switch category {
	case provider.CategoryCloud:
		return "Cloud"
	case provider.CategoryAITool:
		return "Tool"
	case provider.CategoryAIGateway:
		return "Gateway"
	}
	return "Provider"
}

func (f *accountForm) maskedInput(field formField, focused bool) string {
	masked := newLineInput("")
	masked.SetValue(strings.Repeat("•", len([]rune(field.input.Value()))))
	return masked.View(focused)
}

func (f *accountForm) buttons(item formItem, info manager.ProviderInfo, g glyphs, st styles) string {
	enabled := !f.createDisabled()
	var parts []string
	submitLabel := "Create"
	switch {
	case f.editing != nil:
		submitLabel = "Save"
	case info.CanLogin:
		parts = append(parts, renderButton("Create and login", item.kind == itemLogin, enabled, st))
		submitLabel = "Create only"
	}
	parts = append(parts, renderButton(submitLabel, item.kind == itemSubmit, enabled, st))
	if info.CanTest && enabled {
		parts = append(parts, renderButton("Test again", item.kind == itemTest, true, st))
	}
	parts = append(parts, renderPlainButton("Cancel", item.kind == itemCancel, st))
	return strings.Join(parts, " ")
}

// testLine is the outcome of the last connection check, "" before any.
func (f *accountForm) testLine(g glyphs, st styles, width int) string {
	switch {
	case f.test.running:
		return st.Dim.Render("testing" + g.Ellipsis)
	case !f.test.done:
		return ""
	case f.test.err != nil:
		return st.Error.Render(truncateCells(g.No+" "+f.test.err.Error(), width, g.Ellipsis))
	}
	return st.OK.Render(truncateCells(g.Yes+" "+f.test.detail, width, g.Ellipsis))
}

func kindLabel(category provider.Category) string {
	for _, entry := range kindLabels {
		if entry.category == category {
			return entry.label
		}
	}
	return string(category)
}

func cliName(info manager.ProviderInfo) string {
	if info.CLI.Name != "" {
		return info.CLI.Name
	}
	return info.Label + " CLI"
}

func installText(info manager.ProviderInfo) string {
	if info.InstallHint != "" {
		return info.InstallHint
	}
	return "install it, then press R in the list to reload"
}

func radioRow(field formField, g glyphs, st styles, dim bool) string {
	parts := make([]string, len(field.spec.Options))
	for i, option := range field.spec.Options {
		mark := g.RadioOff
		if option == field.value {
			mark = g.RadioOn
		}
		parts[i] = mark + " " + option
		switch {
		case dim || option != field.value:
			parts[i] = st.Dim.Render(parts[i])
		default:
			parts[i] = st.Bold.Render(parts[i])
		}
	}
	return strings.Join(parts, "  ")
}

func wrapText(text string, width int) []string {
	return strings.Split(ansi.Wrap(text, max(width, 10), ""), "\n")
}
