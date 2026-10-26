package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/k0wl0n/gctx/pkg/config"
)

// keyMap is the single source for dispatch, the hint line and the help screen.
type keyMap struct {
	Down, Up, First, Last, PageDown, PageUp key.Binding
	NextTab, PrevTab, JumpTab, JumpLinked   key.Binding
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
	JumpLinked: key.NewBinding(key.WithKeys(">"), key.WithHelp(">", "jump to linked account")),

	Switch:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch to selected account")),
	Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Clear:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear applied filter")),
	Details: key.NewBinding(key.WithKeys("d", "i"), key.WithHelp("d/i", "details")),
	Login:   key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "login")),
	LoginStale: key.NewBinding(key.WithKeys("L"),
		key.WithHelp("L", "login every stale account")),
	Shell:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "subshell, every active slot")),
	ShellOne: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "subshell, this account only")),
	CopyEnv:  key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy the env eval line")),
	Run:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run provider CLI command")),
	Create:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create account")),
	Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit account")),
	Delete:   key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete account")),
	Save:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "save current ADC here")),

	TogglePane:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "toggle detail pane")),
	Fold:        key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "fold or unfold provider group")),
	NeedsAction: key.NewBinding(key.WithKeys("!"), key.WithHelp("!", "only rows needing action")),
	Sort:        key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "cycle name/status/expiry sort")),

	Reload:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	Command: key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "command palette")),
}

// groups returns the bindings by help-screen section.
func (k keyMap) groups() []keyGroup {
	return []keyGroup{
		{"Navigation", []key.Binding{k.Down, k.Up, k.First, k.Last, k.PageDown, k.PageUp, k.NextTab, k.PrevTab, k.JumpTab, k.JumpLinked}},
		{"View", []key.Binding{k.TogglePane, k.Fold, k.NeedsAction, k.Sort}},
		{"Actions", []key.Binding{k.Switch, k.Filter, k.Clear, k.Details, k.Login, k.LoginStale, k.Shell, k.ShellOne, k.CopyEnv, k.Run, k.Create, k.Edit, k.Delete, k.Save}},
		{"General", []key.Binding{k.Reload, k.Help, k.Quit, k.Command}},
	}
}

// helpColumns regroups the same bindings into the help screen's three columns.
func (k keyMap) helpColumns() []keyGroup {
	return []keyGroup{
		{"Navigation", []key.Binding{k.Down, k.Up, k.First, k.Last, k.PageDown, k.PageUp, k.NextTab, k.PrevTab, k.JumpTab, k.JumpLinked, k.Filter, k.Clear, k.Command}},
		{"Account", []key.Binding{k.Switch, k.Details, k.Login, k.LoginStale, k.Create, k.Edit, k.Delete, k.Save, k.CopyEnv}},
		{"Session", []key.Binding{k.Shell, k.ShellOne, k.Run, k.Reload, k.TogglePane, k.Fold, k.NeedsAction, k.Sort, k.Help, k.Quit}},
	}
}

// hint is one hint-line entry: the binding's first key and a short label.
// rank decides what goes first when the line does not fit: the highest rank
// is dropped first, rank 0 is never dropped.
type hint struct {
	key   string
	label string
	rank  int
}

// hints lists the hint line in display order for a tier; renderHints drops
// the lowest-priority entries until the rest fit.
func (k keyMap) hints(t tier, allTab bool, g glyphs) []hint {
	first := func(b key.Binding) string {
		if b.Keys()[0] == "enter" {
			return g.Enter
		}
		return b.Keys()[0]
	}
	help, quit := hint{first(k.Help), "help", 0}, hint{first(k.Quit), "quit", 0}
	if t == tierNarrow {
		return []hint{
			{first(k.NextTab), "provider", 1}, {first(k.Switch), "activate", 3}, {first(k.Filter), "filter", 4},
			help, quit,
		}
	}
	out := []hint{
		{first(k.NextTab), "provider", 1},
		{first(k.Down) + "/" + first(k.Up), "move", 2},
		{first(k.Switch), "activate", 3},
		{first(k.Filter), "filter", 4},
	}
	if t != tierWide {
		out = append(out, hint{first(k.Details), "details", 9})
	}
	out = append(out,
		hint{first(k.Login), "login", 5}, hint{first(k.LoginStale), "login all stale", 8},
		hint{first(k.Create), "create", 7}, hint{first(k.Shell), "shell", 6}, hint{first(k.CopyEnv), "copy env", 10})
	if allTab {
		out = append(out, hint{first(k.Fold), "fold group", 11})
	}
	if t == tierWide {
		out = append(out, hint{first(k.TogglePane), "pane", 13})
	}
	return append(out,
		hint{first(k.NeedsAction), "needs action", 14}, hint{first(k.Sort), "sort", 15}, hint{first(k.Reload), "recheck", 12},
		help, quit)
}

// bindingCLI maps a binding's first key to its CLI equivalent, "<name>"
// standing for the account. Only commands that exist belong here; a binding
// without a command is a TUI-only action. The palette and the help screen
// both read this table.
var bindingCLI = map[string]string{
	"enter":  "switch <name>",
	"l":      "login <name>",
	"S":      "shell <name>",
	"y":      "env <name>",
	"r":      "run <name> -- ...",
	"d":      "info <name>",
	"c":      "create ...",
	"ctrl+d": "delete <name>",
	"a":      "save <name>",
}

// cliEquivalent is the command line equivalent to binding for account, or ""
// when the binding has none.
func cliEquivalent(binding key.Binding, account string) string {
	template, ok := bindingCLI[binding.Keys()[0]]
	if !ok {
		return ""
	}
	return config.AppName + " " + strings.ReplaceAll(template, "<name>", account)
}
