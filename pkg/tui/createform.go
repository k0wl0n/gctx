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
	items = append(items, formItem{kind: itemName})
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

type formLine struct {
	text    string
	focused bool
}

func (f *accountForm) view(m Model) string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	width := max(m.width, minWidth)
	check := f.check()
	item := f.focused()
	values := f.values()

	labelWidth := len("Provider")
	for _, field := range f.fields {
		labelWidth = max(labelWidth, ansi.StringWidth(field.spec.Label))
	}
	labelWidth = min(labelWidth, 18)
	valueIndent := formIndent + 2 + labelWidth + 2
	valueWidth := max(width-valueIndent, 10)

	var lines []formLine
	add := func(text string, focused bool) { lines = append(lines, formLine{text, focused}) }
	row := func(label, value string, focused, dim bool) {
		caret := "  "
		if focused {
			caret = g.Caret + " "
		}
		label = ansi.Truncate(label, labelWidth, "")
		pad := strings.Repeat(" ", labelWidth-ansi.StringWidth(label))
		labelText := label + pad
		if dim {
			labelText = st.Dim.Render(labelText)
		} else if focused {
			labelText = st.Bold.Render(labelText)
		}
		add(strings.Repeat(" ", formIndent)+caret+labelText+"  "+value, focused)
	}
	under := func(text string, style lipgloss.Style) {
		for _, wrapped := range wrapText(text, valueWidth) {
			add(strings.Repeat(" ", valueIndent)+style.Render(wrapped), false)
		}
	}

	title := "Create account"
	if f.editing != nil {
		title = "Edit account"
	}
	add(st.Title.Render(title), false)
	add("", false)

	info, hasProvider := f.info()
	if !hasProvider {
		add(st.Dim.Render("No providers are available."), false)
		add("", false)
	}
	if f.editing != nil {
		row("Provider", info.Label, false, true)
	} else if hasProvider {
		var kindNames []string
		for _, kind := range f.kinds {
			kindNames = append(kindNames, kindLabel(kind))
		}
		row("Kind", choiceRow(kindNames, f.kind, st), item.kind == itemKind, false)
		var providerNames []string
		for _, candidate := range f.providers {
			providerNames = append(providerNames, candidate.Label)
		}
		row("Provider", choiceRow(providerNames, f.provider, st)+st.Dim.Render("  ←/→"), item.kind == itemProvider, false)
	}

	if hasProvider {
		focusedName := item.kind == itemName
		row("Name", f.name.View(focusedName), focusedName, false)
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
		var value string
		switch {
		case field.isOption():
			value = radioRow(field, g, st, readOnly)
		case field.spec.Secret && secretref.LooksLikeKey(field.current()):
			value = f.maskedInput(field, focused)
		case readOnly:
			value = st.Dim.Render(field.input.View(false))
		default:
			value = field.input.View(focused)
		}
		help := field.spec.Help
		if readOnly {
			help = ""
		}
		inline := help != "" && !field.isOption() &&
			ansi.StringWidth(value)+3+ansi.StringWidth(help) <= valueWidth
		if inline {
			value += st.Dim.Render("  " + g.Dot + " " + help)
		}
		row(field.spec.Label, value, focused, readOnly)
		if msg := check.fields[field.spec.Key]; msg != "" {
			under(g.No+" "+msg, st.Error)
		} else if help != "" && !inline {
			under(help, st.Dim)
		}
	}

	if hasProvider {
		if f.createDisabled() {
			add("", false)
			add(strings.Repeat(" ", formIndent)+st.Error.Render(g.No+" "+cliName(info)+" not found: create is disabled"), false)
			for _, wrapped := range wrapText(installText(info), width-formIndent-2) {
				add(strings.Repeat(" ", formIndent+2)+st.Dim.Render(wrapped), false)
			}
		}
		if info.Footer != nil && f.editing == nil {
			writes, never := info.Footer(values)
			add("", false)
			for _, footer := range []struct{ label, text string }{{"writes", writes}, {"never ", never}} {
				for n, wrapped := range wrapText(footer.text, width-formIndent-7) {
					label := strings.Repeat(" ", 6)
					if n == 0 {
						label = footer.label
					}
					add(strings.Repeat(" ", formIndent)+st.Dim.Render(label+" "+wrapped), false)
				}
			}
		}
	}

	add("", false)
	add(strings.Repeat(" ", formIndent)+f.buttons(item, info, g, st), item.kind >= itemLogin)
	hint := strings.Join([]string{"tab next", "shift+tab back", "←/→ change", "esc cancel"}, " "+g.Dot+" ")
	add(strings.Repeat(" ", formIndent)+st.Dim.Render(hint), false)

	return renderFormLines(lines, width, m.height)
}

func (f *accountForm) maskedInput(field formField, focused bool) string {
	masked := newLineInput("")
	masked.SetValue(strings.Repeat("•", len([]rune(field.input.Value()))))
	return masked.View(focused)
}

func (f *accountForm) buttons(item formItem, info manager.ProviderInfo, g glyphs, st styles) string {
	button := func(label string, kind formItemKind, enabled bool) string {
		switch {
		case !enabled:
			return st.Dim.Render("[ " + label + " ]")
		case item.kind == kind:
			return st.TabActive.Render("[*" + label + "*]")
		}
		return "[ " + label + " ]"
	}
	enabled := !f.createDisabled()
	var parts []string
	submitLabel := "Create"
	switch {
	case f.editing != nil:
		submitLabel = "Save"
	case info.CanLogin:
		parts = append(parts, button("Create and login", itemLogin, enabled))
		submitLabel = "Create only"
	}
	parts = append(parts, button(submitLabel, itemSubmit, enabled))
	cancel := "  Cancel  "
	if item.kind == itemCancel {
		cancel = st.TabActive.Render("[*Cancel*]")
	}
	return strings.Join(append(parts, cancel), "  ")
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

// choiceRow draws names with the selected one highlighted.
func choiceRow(names []string, selected int, st styles) string {
	parts := make([]string, len(names))
	for i, name := range names {
		if i == selected {
			parts[i] = st.TabActive.Render(" " + name + " ")
		} else {
			parts[i] = " " + name + " "
		}
	}
	return strings.Join(parts, " ")
}

func radioRow(field formField, g glyphs, st styles, dim bool) string {
	parts := make([]string, len(field.spec.Options))
	for i, option := range field.spec.Options {
		mark := g.RadioOff
		if option == field.value {
			mark = g.RadioOn
		}
		parts[i] = mark + " " + option
		if dim {
			parts[i] = st.Dim.Render(parts[i])
		}
	}
	return strings.Join(parts, "  ")
}

func wrapText(text string, width int) []string {
	return strings.Split(ansi.Wrap(text, max(width, 10), ""), "\n")
}

// renderFormLines clips every line to width and scrolls to keep the focused
// line visible when the form is taller than the terminal.
func renderFormLines(lines []formLine, width, height int) string {
	if height > 0 && len(lines) > height {
		lines = slices.DeleteFunc(slices.Clone(lines), func(line formLine) bool { return line.text == "" })
	}
	if height > 0 && len(lines) > height {
		focus := 0
		for i, line := range lines {
			if line.focused {
				focus = i
				break
			}
		}
		start := max(min(focus-height/2, len(lines)-height), 0)
		lines = lines[start : start+height]
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = ansi.Truncate(line.text, width, "…")
	}
	return strings.Join(out, "\n")
}
