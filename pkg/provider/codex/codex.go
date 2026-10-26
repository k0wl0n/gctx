// Package codex implements the provider contract for the Codex CLI.
//
// Each account owns a private CODEX_HOME, which must exist before codex runs.
// `codex login status` (exit 0 logged in, 1 not) is the only credential
// check; auth.json and the other files codex keeps there are never opened.
package codex

import (
	"context"
	"errors"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/aitool"
	"github.com/k0wl0n/gctx/pkg/provider/providerutil"
)

// ErrCLINotFound reports that the codex CLI is not on PATH.
var ErrCLINotFound = errors.New("codex CLI not found; install Codex")

// notLoggedIn marks the logged-out answer of `codex login status`.
var notLoggedIn = []string{"Not logged in"}

// Options injects directory roots and process hooks.
type Options = aitool.Options

// Provider is the Codex backend.
type Provider struct {
	*aitool.Tool
}

func New(opts Options) *Provider {
	return &Provider{Tool: aitool.NewTool(aitool.Spec{
		ID:          provider.Codex,
		Label:       "Codex",
		TargetName:  "Workspace",
		CLIName:     "codex",
		EnvVar:      "CODEX_HOME",
		DefaultDir:  ".codex",
		LoginArgs:   []string{"login"},
		InstallHint: "install Codex: npm install -g @openai/codex",
		NotFound:    ErrCLINotFound,
	}, opts)}
}

func (p *Provider) TargetLabel() string { return "WORKSPACE" }

func (p *Provider) Badge() provider.Badge {
	return provider.Badge{Text: "OAI", Style: provider.BadgeOutline, Color: "#10a37f"}
}

func (p *Provider) Form() []provider.FormField {
	return []provider.FormField{
		{Key: provider.FormTargetKey, Label: "Workspace label", Help: "optional, display only", Editable: true, Validate: p.ValidateTarget},
	}
}

// CheckAuth runs `codex login status` in every account's directory.
func (p *Provider) CheckAuth(ctx context.Context, accts []config.Account) provider.AuthReport {
	return p.CheckEach(ctx, accts, func(ctx context.Context, runner provider.Runner, env []string) provider.AuthResult {
		if _, err := runner.Query(ctx, env, "login", "status"); err != nil {
			return providerutil.FailedCheck(err, notLoggedIn)
		}
		return provider.AuthResult{Credentialed: true}
	})
}
