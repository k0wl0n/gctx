package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/k0wl0n/gctx/pkg/manager"
)

type loginOutcome int

const (
	loginSucceeded loginOutcome = iota
	loginSkipped
	loginFailed
)

// loginQueue is the logins in progress. A single login is a queue of one.
type loginQueue struct {
	entries  []string // login targets in list order
	next     int      // entry in progress
	loggedIn int
	skipped  int
	failed   int
	last     string // last account handled; selected when the queue ends
}

func (q *loginQueue) current() string {
	if q.next >= len(q.entries) {
		return ""
	}
	return q.entries[q.next]
}

// upcoming is the entries after the one in progress.
func (q *loginQueue) upcoming() []string {
	if q.next+1 >= len(q.entries) {
		return nil
	}
	return q.entries[q.next+1:]
}

func (q *loginQueue) record(outcome loginOutcome) {
	switch outcome {
	case loginSucceeded:
		q.loggedIn++
	case loginSkipped:
		q.skipped++
	case loginFailed:
		q.failed++
	}
}

func (q *loginQueue) summary() (string, statusLevel) {
	if len(q.entries) == 1 {
		name := q.entries[0]
		switch {
		case q.loggedIn > 0:
			return "logged in " + name, levelInfo
		case q.failed > 0:
			return "login failed for " + name, levelWarning
		}
		return "login cancelled for " + name + "; account unchanged", levelInfo
	}
	parts := []string{fmt.Sprintf("logged in %d of %d", q.loggedIn, len(q.entries))}
	if q.skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", q.skipped))
	}
	level := levelInfo
	if q.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", q.failed))
		level = levelWarning
	}
	return strings.Join(parts, ", "), level
}

// loginTarget is the account whose login a row needs: the cloud account a
// linked row runs on (settings.via), else the row itself.
func (m Model) loginTarget(account manager.AccountStatus) string {
	if via := account.Settings["via"]; via != "" && via != account.Name {
		if _, ok := m.overview.Account(via); ok {
			return via
		}
	}
	return account.Name
}

// staleLoginTargets lists, in list order and once each, the login targets of
// every account that needs a login, whatever the filter and tab show.
func (m Model) staleLoginTargets() []string {
	var targets []string
	seen := map[string]bool{}
	for _, account := range m.overview.Accounts {
		if account.Status != manager.HealthAuthRequired && account.Status != manager.HealthExpired {
			continue
		}
		if target := m.loginTarget(account); !seen[target] {
			seen[target] = true
			targets = append(targets, target)
		}
	}
	return targets
}

// startLoginQueue logs in every stale account, one after the other.
func (m Model) startLoginQueue() (Model, tea.Cmd) {
	if m.busy() {
		return m, m.setStatus("Busy: "+m.op.desc, levelWarning)
	}
	targets := m.staleLoginTargets()
	if len(targets) == 0 {
		return m, m.setStatus("no account needs login", levelInfo)
	}
	m.queue = &loginQueue{entries: targets}
	return m.runQueueEntry()
}

// runQueueEntry starts the entry in progress, passing over entries that
// cannot log in, and ends the queue after the last one.
func (m Model) runQueueEntry() (Model, tea.Cmd) {
	q := m.queue
	for q.current() != "" {
		name := q.current()
		account, ok := m.overview.Account(name)
		info, known := m.knownProvider(account.Provider)
		if !ok || (known && (!info.CanLogin || !info.CLI.Found)) {
			q.skipped++
			q.next++
			continue
		}
		if info.StreamsLogin {
			return m.startStreamLogin(name)
		}
		return m.beginExecLogin(name)
	}
	return m.endQueue()
}

// leaveLogin records the outcome of the entry in progress and moves on, or
// ends the queue (the rest counts as skipped) when abort is set.
func (m Model) leaveLogin(outcome loginOutcome, abort bool) (Model, tea.Cmd) {
	q := m.queue
	q.record(outcome)
	q.last = q.current()
	q.next++
	if abort {
		q.skipped += len(q.entries) - q.next
		q.next = len(q.entries)
	}
	m.closeLoginSession()
	m.reload()
	return m.runQueueEntry()
}

func (m Model) endQueue() (Model, tea.Cmd) {
	q := m.queue
	m.queue = nil
	m.closeLoginSession()
	m.screen = screenMain
	m.reload()
	if q.last != "" && m.isVisible(q.last) {
		m.selected = q.last
	}
	text, level := q.summary()
	cmds := []tea.Cmd{m.setStatus(text, level)}
	if q.loggedIn > 0 {
		cmds = append(cmds, m.startCredsCheck())
	}
	return m, tea.Batch(cmds...)
}
