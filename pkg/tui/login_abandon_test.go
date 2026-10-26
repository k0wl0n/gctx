package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestLoginNextReturnsAfterAbandon pins that a pending next() command returns
// once the session is abandoned instead of leaking its goroutine.
func TestLoginNextReturnsAfterAbandon(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &loginSession{id: 7, ctx: ctx, cancel: cancel, events: make(chan tea.Msg), abandoned: make(chan struct{})}
	s.start.Do(func() {}) // the login itself never sends
	done := make(chan tea.Msg, 1)
	go func() { done <- s.next()() }()

	m := Model{login: s}
	m.closeLoginSession()
	select {
	case msg := <-done:
		if msg != nil {
			t.Fatalf("abandoned next() = %#v, want nil", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("next() still blocked after the session was abandoned")
	}
}
