package tui

import "charm.land/bubbles/v2/key"

// keyMap is the single source for dispatch, the hint line and the help screen.
type keyMap struct {
	Down, Up, First, Last, PageDown, PageUp key.Binding
	NextTab, PrevTab, JumpTab               key.Binding
	Switch, Filter, Clear, Details          key.Binding
	Login, LoginStale, Shell, ShellOne      key.Binding
	CopyEnv, Run                            key.Binding
	Create, Edit, Delete, Save              key.Binding
	TogglePane, Fold, NeedsAction, Sort     key.Binding
	Reload, Help, Quit, Command             key.Binding
}

// keyGroup is a titled section of the help screen.
type keyGroup struct {
	Title    string
	Bindings []key.Binding
}

var keys = keyMap{
	Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "next row")),
	Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "previous row")),
	First:    key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g/home", "first row")),
	Last:     key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G/end", "last row")),
	PageDown: key.NewBinding(key.WithKeys("ctrl+f", "pgdown"), key.WithHelp("ctrl+f/pgdn", "page down")),
	PageUp:   key.NewBinding(key.WithKeys("ctrl+b", "pgup"), key.WithHelp("ctrl+b/pgup", "page up")),
	NextTab:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next provider tab")),
	PrevTab:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous provider tab")),
	JumpTab: key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"),
		key.WithHelp("1-9", "jump to tab (1 is All)")),

	Switch:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch to selected account")),
	Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Clear:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear applied filter")),
	Details: key.NewBinding(key.WithKeys("d", "i"), key.WithHelp("d/i", "details")),
	Login:   key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "login")),
	LoginStale: key.NewBinding(key.WithKeys("L"),
		key.WithHelp("L", "login every account that needs it (all providers; ignores the filter and tab)")),
	Shell:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "subshell with every active slot")),
	ShellOne: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "subshell with only this account")),
	CopyEnv:  key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy the env eval line")),
	Run:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run gcloud command")),
	Create:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create account")),
	Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit account")),
	Delete:   key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete account")),
	Save:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "save current ADC to selected account")),

	TogglePane:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "toggle detail pane (150+ columns)")),
	Fold:        key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "fold or unfold the provider group")),
	NeedsAction: key.NewBinding(key.WithKeys("!"), key.WithHelp("!", "only rows needing action")),
	Sort:        key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "cycle sort: name, status, expiry")),

	Reload:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	Command: key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "reserved: command mode")),
}

// groups returns the bindings by help-screen section.
func (k keyMap) groups() []keyGroup {
	return []keyGroup{
		{"Navigation", []key.Binding{k.Down, k.Up, k.First, k.Last, k.PageDown, k.PageUp, k.NextTab, k.PrevTab, k.JumpTab}},
		{"Actions", []key.Binding{k.Switch, k.Filter, k.Clear, k.Details, k.Login, k.LoginStale, k.Shell, k.ShellOne, k.CopyEnv, k.Run, k.Create, k.Edit, k.Delete, k.Save}},
		{"View", []key.Binding{k.TogglePane, k.Fold, k.NeedsAction, k.Sort}},
		{"General", []key.Binding{k.Reload, k.Help, k.Quit, k.Command}},
	}
}

// hint is one hint-line entry: the binding's first key and a short label.
type hint struct {
	key   string
	label string
}

// hints lists the hint line in priority order for a tier; renderHints keeps
// as many as fit.
func (k keyMap) hints(t tier, allTab bool, g glyphs) []hint {
	first := func(b key.Binding) string {
		if b.Keys()[0] == "enter" {
			return g.Enter
		}
		return b.Keys()[0]
	}
	if t == tierNarrow {
		return []hint{
			{first(k.Switch), "activate"}, {first(k.Filter), "filter"}, {first(k.Details), "details"},
			{first(k.NextTab), "provider"}, {first(k.Help), "help"},
		}
	}
	out := []hint{
		{first(k.NextTab), "provider"},
		{first(k.Down) + "/" + first(k.Up), "move"},
		{first(k.Switch), "activate"},
		{first(k.Filter), "filter"},
	}
	if t != tierWide {
		out = append(out, hint{first(k.Details), "details"})
	}
	out = append(out, hint{first(k.Login), "login"}, hint{first(k.LoginStale), "login all stale"}, hint{first(k.Create), "create"}, hint{first(k.Shell), "shell"}, hint{first(k.CopyEnv), "copy env"})
	if allTab {
		out = append(out, hint{first(k.Fold), "fold group"})
	}
	if t == tierWide {
		out = append(out, hint{first(k.TogglePane), "pane"})
	}
	return append(out,
		hint{first(k.NeedsAction), "needs action"}, hint{first(k.Sort), "sort"}, hint{first(k.Reload), "recheck"},
		hint{first(k.Help), "help"}, hint{first(k.Quit), "quit"})
}
