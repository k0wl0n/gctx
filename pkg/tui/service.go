package tui

import (
	"context"
	"os/exec"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// Service is everything the TUI needs from the domain layer. *manager.Manager
// implements it; tests use an in-memory fake. Validation lives in
// manager.ValidateAccountName and manager.ValidateProjectID, which are pure.
type Service interface {
	Overview() (manager.Overview, error)
	CheckCredentials(ctx context.Context) (manager.CredentialReport, error)
	CheckProviderCredentials(ctx context.Context, id provider.ID) (manager.CredentialReport, error)
	Providers() []manager.ProviderInfo
	Env(name string) ([]string, error) // env lines; key references, never key values
	Info(name string) (manager.AccountStatus, error)
	Switch(ctx context.Context, name string) (manager.SwitchResult, error)
	Create(ctx context.Context, name, projectID string) (manager.CreateResult, error)
	Login(ctx context.Context, name string, io manager.LoginIO) (manager.LoginResult, error)
	SaveCredentials(ctx context.Context, name string) (manager.SaveResult, error)
	Delete(ctx context.Context, name string, opt manager.DeleteOptions) (manager.DeleteResult, error)
	RunCommand(name string, args []string) (*exec.Cmd, error)
	ShellCommand(name string) (*exec.Cmd, error)
	GcloudBin() string
}

var _ Service = (*manager.Manager)(nil)
