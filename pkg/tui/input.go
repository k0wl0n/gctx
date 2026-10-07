package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// lineInput is a minimal single-line text field. It replaces
// bubbles/textinput, which links a system-clipboard package that the TUI must
// not depend on (credential-safety: no clipboard access).
type lineInput struct {
	Prompt string
	value  []rune
	cursor int
}

func newLineInput(prompt string) lineInput { return lineInput{Prompt: prompt} }

func (i lineInput) Value() string { return string(i.value) }

func (i lineInput) Cursor() int { return i.cursor }

func (i *lineInput) SetValue(s string) {
	i.value = []rune(s)
	i.cursor = len(i.value)
}

func (i *lineInput) Reset() { i.SetValue("") }

// update applies an editing key and reports whether the key was consumed.
// Command keys (enter, esc, tab, up, down, ctrl+*) are left to the caller.
func (i *lineInput) update(msg tea.KeyPressMsg) bool {
	switch key := msg.String(); key {
	case "left":
		i.cursor = max(i.cursor-1, 0)
	case "right":
		i.cursor = min(i.cursor+1, len(i.value))
	case "home", "ctrl+a":
		i.cursor = 0
	case "end", "ctrl+e":
		i.cursor = len(i.value)
	case "backspace":
		if i.cursor > 0 {
			i.value = append(i.value[:i.cursor-1], i.value[i.cursor:]...)
			i.cursor--
		}
	case "delete":
		if i.cursor < len(i.value) {
			i.value = append(i.value[:i.cursor], i.value[i.cursor+1:]...)
		}
	case "ctrl+u":
		i.value = i.value[i.cursor:]
		i.cursor = 0
	default:
		text := msg.Key().Text
		if text == "" || msg.Key().Mod&(tea.ModCtrl|tea.ModAlt) != 0 {
			return false
		}
		runes := []rune(text)
		i.value = append(i.value[:i.cursor], append(runes, i.value[i.cursor:]...)...)
		i.cursor += len(runes)
	}
	return true
}

// View renders the prompt, value and a block cursor when focused.
func (i lineInput) View(focused bool) string {
	if !focused {
		return i.Prompt + string(i.value)
	}
	var b strings.Builder
	b.WriteString(i.Prompt)
	b.WriteString(string(i.value[:i.cursor]))
	if i.cursor < len(i.value) {
		b.WriteString("\x1b[7m" + string(i.value[i.cursor]) + "\x1b[27m")
		b.WriteString(string(i.value[i.cursor+1:]))
	} else {
		b.WriteString("\x1b[7m \x1b[27m")
	}
	return b.String()
}
