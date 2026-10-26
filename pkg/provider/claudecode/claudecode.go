// Package claudecode implements the provider contract for Claude Code.
//
// Each account owns a private CLAUDE_CONFIG_DIR. `claude auth status` (JSON,
// exit 0 logged in, 1 logged out) is the only credential check; the files
// Claude Code keeps in that directory are never opened. An account may run
// on a GCP account through Vertex AI or an AWS account through Bedrock
// instead of its own login (settings.via, see LinkEnv).
package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/aitool"
)

const (
	envConfigDir = "CLAUDE_CONFIG_DIR"

	authSubscription = "subscription"
	authVia          = "via cloud account"

	backendVertex  = "vertex"
	backendBedrock = "bedrock"
)

// ErrCLINotFound reports that the claude CLI is not on PATH.
var ErrCLINotFound = errors.New("claude CLI not found; install Claude Code")

// Options injects directory roots and process hooks.
type Options = aitool.Options

// Provider is the Claude Code backend.
type Provider struct {
	*aitool.Tool
}

func New(opts Options) *Provider {
	return &Provider{Tool: aitool.NewTool(aitool.Spec{
		ID:          provider.ClaudeCode,
		Label:       "Claude Code",
		TargetName:  "Org",
		CLIName:     "claude",
		EnvVar:      envConfigDir,
		DefaultDir:  ".claude",
		LoginArgs:   []string{"auth", "login"},
		InstallHint: "install Claude Code: npm install -g @anthropic-ai/claude-code",
		NotFound:    ErrCLINotFound,
	}, opts)}
}

func (p *Provider) TargetLabel() string { return "ORG" }

func (p *Provider) Badge() provider.Badge {
	return provider.Badge{Text: "CLD", Style: provider.BadgeOutline, Color: "#d97757"}
}

func (p *Provider) Columns() []provider.Column {
	return []provider.Column{{Key: provider.SettingVia, Header: "VIA", Width: 16, Value: func(v provider.AccountView) string {
		if via := v.Settings[provider.SettingVia]; via != "" {
			return via + " (" + v.Settings[provider.SettingBackend] + ")"
		}
		return ""
	}}}
}

func (p *Provider) Form() []provider.FormField {
	linked := func(values map[string]string) bool { return values["auth"] == authVia }
	return []provider.FormField{
		{Key: provider.FormTargetKey, Label: "Org label", Help: "optional, display only", Editable: true, Validate: p.ValidateTarget},
		{Key: "auth", Label: "Auth", Default: authSubscription, Options: []string{authSubscription, authVia}},
		{Key: provider.SettingVia, Label: "Cloud account", Help: "a GCP or AWS account name", Visible: linked, Editable: true},
		{Key: provider.SettingBackend, Label: "Backend", Default: backendVertex, Options: []string{backendVertex, backendBedrock}, Visible: linked, Editable: true},
		{Key: provider.SettingRegion, Label: "Region", Default: "us-east5", Visible: linked, Editable: true},
	}
}

// Details adds the linked account and backend to the shared rows.
func (p *Provider) Details(acct config.Account) []provider.Field {
	fields := p.Tool.Details(acct)
	if via := acct.Settings[provider.SettingVia]; via != "" {
		fields = append(fields, provider.Field{Label: "Runs on", Value: via + " via " + acct.Settings[provider.SettingBackend] + " in " + acct.Settings[provider.SettingRegion]})
	}
	return fields
}

// LinkEnv returns the env that sends Claude Code through the linked cloud
// account: Vertex AI on GCP, Bedrock on AWS. The backend defaults from the
// linked provider.
func (p *Provider) LinkEnv(acct, linked config.Account, linkedID provider.ID) ([]string, error) {
	own, err := p.Env(acct)
	if err != nil {
		return nil, err
	}
	backend := acct.Settings[provider.SettingBackend]
	if backend == "" {
		switch linkedID {
		case provider.GCP:
			backend = backendVertex
		case provider.AWS:
			backend = backendBedrock
		}
	}
	region := acct.Settings[provider.SettingRegion]
	switch {
	case backend == backendVertex && linkedID == provider.GCP:
		if region == "" {
			region = "us-east5"
		}
		return append(own, "CLAUDE_CODE_USE_VERTEX=1", "ANTHROPIC_VERTEX_PROJECT_ID="+linked.Target(), "CLOUD_ML_REGION="+region), nil
	case backend == backendBedrock && linkedID == provider.AWS:
		if region == "" {
			region = linked.Settings[provider.SettingRegion]
		}
		if region == "" {
			return nil, fmt.Errorf("%s needs a region for Bedrock", acct.Name)
		}
		return append(own, "CLAUDE_CODE_USE_BEDROCK=1", "AWS_REGION="+region), nil
	case backend == backendVertex || backend == backendBedrock:
		return nil, fmt.Errorf("Claude Code cannot use %s through %s account %s: Vertex AI needs a GCP account, Bedrock an AWS account", backend, providerName(linkedID), linked.Name)
	}
	return nil, fmt.Errorf("Claude Code cannot run on %s account %s: supported are GCP (Vertex AI) and AWS (Bedrock)", providerName(linkedID), linked.Name)
}

func providerName(id provider.ID) string {
	switch id {
	case provider.GCP:
		return "GCP"
	case provider.AWS:
		return "AWS"
	case provider.Azure:
		return "Azure"
	}
	return string(id)
}

// authStatus is the subset of `claude auth status` gctx reads; ids and
// tokens are not decoded.
type authStatus struct {
	LoggedIn bool   `json:"loggedIn"`
	Email    string `json:"email"`
}

// CheckAuth runs `claude auth status` in every account's directory.
func (p *Provider) CheckAuth(ctx context.Context, accts []config.Account) provider.AuthReport {
	return p.CheckEach(ctx, accts, func(ctx context.Context, runner provider.Runner, env []string) provider.AuthResult {
		out, err := runner.Query(ctx, env, "auth", "status")
		if err != nil {
			var commandErr *provider.CommandError
			if errors.As(err, &commandErr) {
				if status, ok := parseStatus(commandErr.Output); ok && !status.LoggedIn {
					return provider.AuthResult{}
				}
			}
			return provider.AuthResult{Err: err}
		}
		status, ok := parseStatus(out)
		if !ok {
			return provider.AuthResult{Err: errors.New("claude auth status: unexpected output")}
		}
		return provider.AuthResult{Credentialed: status.LoggedIn, Identity: status.Email}
	})
}

// parseStatus decodes the JSON object in the output, ignoring other lines.
func parseStatus(output string) (authStatus, bool) {
	start, end := strings.Index(output, "{"), strings.LastIndex(output, "}")
	if start < 0 || end < start {
		return authStatus{}, false
	}
	var status authStatus
	if err := json.Unmarshal([]byte(output[start:end+1]), &status); err != nil {
		return authStatus{}, false
	}
	return status, true
}
