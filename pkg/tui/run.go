package tui

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// Options configures a TUI session.
type Options struct {
	Version string
	Debug   bool
}

// Run starts the TUI and returns after the terminal has been restored. A
// quit, interrupt or cancelled context is a clean exit.
func Run(ctx context.Context, svc Service, opts Options) error {
	model := New(svc, opts)
	_, err := tea.NewProgram(model, tea.WithContext(ctx)).Run()
	return mapRunError(err)
}

func mapRunError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, tea.ErrProgramPanic):
		return fmt.Errorf("interactive UI failed: %w", err)
	case errors.Is(err, tea.ErrInterrupted), errors.Is(err, tea.ErrProgramKilled):
		return nil
	}
	return fmt.Errorf("interactive UI failed: %w", err)
}
