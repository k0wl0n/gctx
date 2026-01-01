package manager

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/k0wl0n/gctx/pkg/adc"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/watcher"
	"github.com/ktr0731/go-fuzzyfinder"
)

type Manager struct {
	config *config.Config
}

func New() (*Manager, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return &Manager{config: cfg}, nil
}

// SelectAccountInteractive launches an interactive UI to select an account
func (m *Manager) SelectAccountInteractive() (string, error) {
	accounts := m.config.ListAccounts()
	if len(accounts) == 0 {
		return "", fmt.Errorf("no accounts configured")
	}

	idx, err := fuzzyfinder.Find(
		accounts,
		func(i int) string {
			acc := accounts[i]
			active := ""
			if acc.Name == m.config.ActiveAccount {
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

// CreateAccount creates a new account with optional auto-save
func (m *Manager) CreateAccount(name, projectID string, autoSave bool) error {
	configName := fmt.Sprintf("%s-config", name)

	// Create gcloud config
	if err := gcloud.CreateConfig(configName); err != nil {
		return err
	}
	fmt.Printf("Created gcloud configuration: %s\n", configName)

	// Activate and set project
	if err := gcloud.ActivateConfig(configName); err != nil {
		return err
	}

	if err := gcloud.SetProject(projectID); err != nil {
		return err
	}
	fmt.Printf("Set project: %s\n", projectID)

	// Add to config
	account := &config.Account{
		Name:       name,
		ConfigName: configName,
		ProjectID:  projectID,
		CreatedAt:  time.Now(),
	}

	if err := m.config.AddAccount(account); err != nil {
		return err
	}
	fmt.Printf("Account '%s' added to configuration.\n\n", name)

	if autoSave {
		return m.autoSaveFlow(name)
	}

	// Manual flow
	fmt.Println("Now run the following commands:")
	fmt.Println("  1. gcloud auth login")
	fmt.Println("  2. gcloud auth application-default login")
	fmt.Printf("  3. gctx save %s\n", name)

	return nil
}

// Login runs authentication flow for an existing account
func (m *Manager) Login(name string) error {
	// Check if account exists
	_, err := m.config.GetAccount(name)
	if err != nil {
		return err
	}

	// Switch to the account first to ensure we are updating the right gcloud config
	if err := m.SwitchAccount(name); err != nil {
		return fmt.Errorf("failed to switch to account before login: %w", err)
	}

	return m.autoSaveFlow(name)
}

func (m *Manager) autoSaveFlow(accountName string) error {
	fmt.Println("Running authentication...")

	// Run gcloud auth login
	if err := gcloud.AuthLogin(); err != nil {
		return fmt.Errorf("auth login failed: %w", err)
	}
	fmt.Println("Logged in successfully.")

	// Run gcloud auth application-default login
	fmt.Println("Running ADC authentication...")
	// Start watching before triggering auth to ensure we catch the file creation/update
	// However, auth is interactive, so we can't block here.
	// The original design had WatchADC *after* starting auth command but `AuthADCLogin` blocks.
	// But `AuthADCLogin` is interactive (opens browser).
	// Let's rely on `watcher.WatchADC` being called *after* `AuthADCLogin` returns?
	// Wait, `AuthADCLogin` waits for the command to finish. When it finishes, the file should be there.
	// So strictly speaking, we might not need a watcher if `gcloud` guarantees the file is written before it exits.
	// But let's follow the architecture: maybe the file takes a moment to appear or we want to verify it.
	// Actually, `AuthADCLogin` blocks until the user completes the flow in the browser.
	// So when it returns, the file *should* be there.
	// The `watcher` might be useful if we run `AuthADCLogin` in background or if we want to be extra sure.
	// In the provided architecture `watcher.WatchADC` is called *after* `AuthADCLogin`.
	// This implies we are just verifying the file was created/updated.

	warnings, err := gcloud.AuthADCLogin()
	if err != nil {
		return fmt.Errorf("ADC auth failed: %w", err)
	}

	// Watch for ADC file (verification)
	// Since the command finished, we just check if it's there and valid.
	// But let's use the watcher as requested, maybe with a short timeout since it should be immediate.
	if err := watcher.WatchADC(5 * time.Second); err != nil {
		// If watcher fails, it might mean the file wasn't updated or created.
		// But let's try to proceed anyway if the file exists.
		fmt.Printf("Watcher warning: %v\n", err)
	}

	// Auto-save
	adcPath, err := adc.SaveADC(accountName)
	if err != nil {
		return err
	}

	// Update config with ADC path and email
	account, _ := m.config.GetAccount(accountName)
	account.ADCPath = adcPath
	account.Email, _ = adc.GetADCEmail(adc.GetDefaultADCPath())
	m.config.Save()

	fmt.Printf("ADC credentials auto-saved for: %s\n", accountName)
	fmt.Printf("Saved to: %s\n\n", adcPath)

	// Show warnings
	if len(warnings) > 0 {
		fmt.Println("Warnings:")
		for _, w := range warnings {
			fmt.Printf("   %s\n", w)
		}
		fmt.Println()
	}

	fmt.Printf("Account '%s' is ready to use!\n", accountName)
	fmt.Printf("Run: gctx switch %s\n", accountName)

	return nil
}

// SwitchAccount switches to a different account
func (m *Manager) SwitchAccount(name string) error {
	account, err := m.config.GetAccount(name)
	if err != nil {
		return err
	}

	// Restore ADC
	if err := adc.RestoreADC(name); err != nil {
		return err
	}

	// Activate gcloud config
	if err := gcloud.ActivateConfig(account.ConfigName); err != nil {
		return err
	}

	// Ensure project ID is set correctly (in case it was changed manually)
	if err := gcloud.SetProject(account.ProjectID); err != nil {
		if strings.Contains(err.Error(), "Reauthentication required") {
			fmt.Printf("\nWarning: Failed to set project ID because re-authentication is required.\n")
			fmt.Printf("Please run: gctx login %s\n\n", name)
		} else {
			fmt.Printf("Warning: failed to set project ID: %v\n", err)
		}
	}

	// Update active account
	m.config.SetActive(name)

	fmt.Printf("Switched to account: %s (%s)\n", name, account.ProjectID)
	return nil
}

// SaveCredentials manually saves current ADC
func (m *Manager) SaveCredentials(name string) error {
	account, err := m.config.GetAccount(name)
	if err != nil {
		return err
	}

	adcPath, err := adc.SaveADC(name)
	if err != nil {
		return err
	}

	account.ADCPath = adcPath
	account.Email, _ = adc.GetADCEmail(adc.GetDefaultADCPath())
	m.config.Save()

	fmt.Printf("ADC credentials saved for: %s\n", name)
	fmt.Printf("Location: %s\n", adcPath)

	return nil
}

// ListAccounts lists all accounts
func (m *Manager) ListAccounts() error {
	accounts := m.config.ListAccounts()

	if len(accounts) == 0 {
		fmt.Println("No accounts configured")
		return nil
	}

	fmt.Println("\nConfigured Accounts:")
	fmt.Println("===================")

	for _, acc := range accounts {
		active := ""
		if acc.Name == m.config.ActiveAccount {
			active = " ← active"
		}

		email := ""
		if acc.Email != "" {
			email = fmt.Sprintf(" [%s]", acc.Email)
		}

		fmt.Printf("  %s (%s)%s%s\n",
			acc.Name, acc.ProjectID, email, active)
	}

	return nil
}

// GetActiveAccount returns the active account
func (m *Manager) GetActiveAccount() (string, error) {
	if m.config.ActiveAccount != "" {
		return m.config.ActiveAccount, nil
	}

	// Try to detect from current ADC
	return "unknown", nil
}

// DeleteAccount removes an account
func (m *Manager) DeleteAccount(name string, deleteGcloudConfig bool) error {
	account, err := m.config.GetAccount(name)
	if err != nil {
		return err
	}

	// Delete ADC file
	if account.ADCPath != "" {
		os.Remove(account.ADCPath)
	}

	// Delete gcloud config if requested
	if deleteGcloudConfig {
		cmd := exec.Command("gcloud", "config", "configurations",
			"delete", account.ConfigName, "--quiet")
		cmd.Run()
	}

	// Remove from config
	m.config.DeleteAccount(name)

	fmt.Printf("Deleted account: %s\n", name)
	return nil
}

// RunWithAccount runs command with specific account
func (m *Manager) RunWithAccount(name string, args []string) error {
	// Switch to account
	if err := m.SwitchAccount(name); err != nil {
		return err
	}

	// Run command
	return gcloud.RunCommand(args...)
}

// ShowAccountInfo displays detailed account info
func (m *Manager) ShowAccountInfo(name string) error {
	account, err := m.config.GetAccount(name)
	if err != nil {
		return err
	}

	fmt.Printf("\nAccount: %s\n", account.Name)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Project ID:       %s\n", account.ProjectID)
	fmt.Printf("Config Name:      %s\n", account.ConfigName)

	if account.Email != "" {
		fmt.Printf("Email:            %s\n", account.Email)
	}

	if account.ADCPath != "" {
		fmt.Printf("ADC Path:         %s\n", account.ADCPath)

		if info, err := os.Stat(account.ADCPath); err == nil {
			fmt.Printf("ADC Last Modified: %s\n",
				info.ModTime().Format("2006-01-02 15:04:05"))
		}
	}

	fmt.Printf("Created:          %s\n",
		account.CreatedAt.Format("2006-01-02 15:04:05"))

	if account.Name == m.config.ActiveAccount {
		fmt.Println("\nThis is the active account.")
	}

	return nil
}
