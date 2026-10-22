package manager

import (
	"fmt"

	"github.com/ktr0731/go-fuzzyfinder"
)

// SelectAccountInteractive launches the fuzzy-finder picker used by
// `gctx switch` and `gctx shell` without an argument.
func (m *Manager) SelectAccountInteractive() (string, error) {
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err != nil {
		return "", err
	}
	accounts := cfg.ListAccounts()
	if len(accounts) == 0 {
		return "", fmt.Errorf("no accounts configured")
	}

	idx, err := fuzzyfinder.Find(
		accounts,
		func(i int) string {
			acc := accounts[i]
			active := ""
			if acc.Name == cfg.ActiveBySlot[acc.ProviderID()] {
				active = " (active)"
			}
			return fmt.Sprintf("%s [%s]%s", acc.Name, acc.ProjectID, active)
		},
		fuzzyfinder.WithPreviewWindow(func(i, w, h int) string {
			if i == -1 {
				return ""
			}
			acc := accounts[i]
			return fmt.Sprintf("Name: %s\nProject: %s\nEmail: %s\nCreated: %s",
				acc.Name,
				acc.ProjectID,
				acc.Email,
				acc.CreatedAt.Format("2006-01-02 15:04:05"),
			)
		}),
	)
	if err != nil {
		return "", err
	}
	return accounts[idx].Name, nil
}
