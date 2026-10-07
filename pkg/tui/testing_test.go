package tui

import (
	"bytes"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// newTestModel builds a model over svc at the given terminal size. The clock
// is fixed; advance it by assigning m.now.
func newTestModel(t testing.TB, svc Service, width, height int) Model {
	t.Helper()
	m := New(svc, Options{Version: "test"})
	m, _ = m.dispatch(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

// keyMsg converts a key name such as "j", "G", "enter", "ctrl+d", "space" or
// "shift+tab" to the message the runtime would deliver.
func keyMsg(name string) tea.KeyPressMsg {
	special := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "backspace": tea.KeyBackspace,
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
	}
	if code, ok := special[name]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	if name == "space" {
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	if name == "shift+tab" {
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	}
	if rest, ok := strings.CutPrefix(name, "ctrl+"); ok && len([]rune(rest)) == 1 {
		return tea.KeyPressMsg{Code: []rune(rest)[0], Mod: tea.ModCtrl}
	}
	r := []rune(name)
	return tea.KeyPressMsg{Code: r[0], Text: name}
}

// press sends keys in order, executing returned commands inline, and returns
// the final model together with the last non-nil command batch result.
func press(t testing.TB, m Model, keys ...string) Model {
	t.Helper()
	for _, name := range keys {
		var cmd tea.Cmd
		m, cmd = m.dispatch(keyMsg(name))
		m = drain(t, m, cmd)
	}
	return m
}

// resize delivers a window size message.
func resize(t testing.TB, m Model, width, height int) Model {
	t.Helper()
	m, _ = m.dispatch(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

const drainLimit = 1000

// drain runs cmd inline, feeding every produced message back into the model
// until no commands remain. Batches and sequences are flattened. Quit and
// timer commands are not executed; quitting is reported through the model's
// returned messages by stopping the drain.
func drain(t testing.TB, m Model, cmd tea.Cmd) Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > drainLimit {
			t.Fatal("drain: step limit exceeded")
		}
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		msg := next()
		switch msg := msg.(type) {
		case nil, tea.QuitMsg:
			continue
		case tea.BatchMsg:
			queue = append(queue, msg...)
			continue
		}
		var follow tea.Cmd
		m, follow = m.dispatch(msg)
		queue = append(queue, follow)
	}
	return m
}

// screen returns the rendered view with escape sequences stripped.
func screen(m Model) string { return ansi.Strip(m.View().Content) }

// execSync is a Model.exec replacement that runs the command synchronously
// against in-memory pipes. input is scripted stdin (for example "\n"); the
// output written by the command is appended to *output when it is non-nil.
func execSync(input string, output *bytes.Buffer) func(tea.ExecCommand, tea.ExecCallback) tea.Cmd {
	return func(c tea.ExecCommand, fn tea.ExecCallback) tea.Cmd {
		return func() tea.Msg {
			var sink io.Writer = io.Discard
			if output != nil {
				sink = output
			}
			c.SetStdin(strings.NewReader(input))
			c.SetStdout(sink)
			c.SetStderr(sink)
			err := c.Run()
			if fn == nil {
				return nil
			}
			return fn(err)
		}
	}
}
