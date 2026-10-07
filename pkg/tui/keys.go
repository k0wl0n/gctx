package tui

import "charm.land/bubbles/v2/key"

// keyMap is the single source for dispatch, the hint line and the help screen.
type keyMap struct {
	Down, Up, First, Last, PageDown, PageUp key.Binding
	Switch, Filter, Clear, Details          key.Binding
	Login, Shell, Run, Create, Delete, Save key.Binding
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

	Switch:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch to selected account")),
	Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Clear:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear applied filter")),
	Details: key.NewBinding(key.WithKeys("d", "i"), key.WithHelp("d/i", "details")),
	Login:   key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "login")),
	Shell:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "isolated shell")),
	Run:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run gcloud command")),
	Create:  key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create account")),
	Delete:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete account")),
	Save:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "save current ADC to selected account")),

	Reload:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	Command: key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "reserved: command mode")),
}

// groups returns the bindings by help-screen section.
func (k keyMap) groups() []keyGroup {
	return []keyGroup{
		{"Navigation", []key.Binding{k.Down, k.Up, k.First, k.Last, k.PageDown, k.PageUp}},
		{"Actions", []key.Binding{k.Switch, k.Filter, k.Clear, k.Details, k.Login, k.Shell, k.Run, k.Create, k.Delete, k.Save}},
		{"General", []key.Binding{k.Reload, k.Help, k.Quit, k.Command}},
	}
}
