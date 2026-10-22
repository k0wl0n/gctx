package aws

import "github.com/k0wl0n/gctx/pkg/provider"

func (p *Provider) Category() provider.Category { return provider.CategoryCloud }

func (p *Provider) Badge() provider.Badge {
	return provider.Badge{Text: "AWS", Style: provider.BadgeFilled, Color: "#ffa657"}
}

func (p *Provider) Slot() string { return string(p.ID()) }

// EnvBased reports that activation only prints an env line (provider.EnvBased).
func (p *Provider) EnvBased() bool { return true }

func (p *Provider) Columns() []provider.Column {
	return []provider.Column{
		{Key: "region", Header: "REGION", Width: 14, Value: func(v provider.AccountView) string { return v.Settings["region"] }},
	}
}

// Form mirrors Create's inputs; the sso_* fields apply to auth=sso only.
// Only region may change after creation.
func (p *Provider) Form() []provider.FormField {
	ssoOnly := func(values map[string]string) bool { return values["auth"] != authProfile }
	return []provider.FormField{
		{Key: "auth", Label: "Auth", Default: authSSO, Options: []string{authSSO, authProfile},
			Help: "sso writes a new profile; profile adopts an existing one"},
		{Key: provider.FormTargetKey, Label: "Profile name", Validate: p.ValidateTarget},
		{Key: "sso_start_url", Label: "SSO start URL", Visible: ssoOnly},
		{Key: "sso_region", Label: "SSO region", Visible: ssoOnly},
		{Key: "region", Label: "Region", Editable: true},
		{Key: "sso_account_id", Label: "SSO account ID", Help: "optional", Visible: ssoOnly},
		{Key: "sso_role_name", Label: "SSO role name", Help: "optional", Visible: ssoOnly},
	}
}

// Footer implements provider.FormFooter.
func (p *Provider) Footer(values map[string]string) (writes, never string) {
	writes = "config.json (name, profile, regions)"
	if values["auth"] != authProfile {
		profile := values[provider.FormTargetKey]
		if profile == "" {
			profile = "<profile>"
		}
		writes += " · ~/.aws/config [profile " + profile + "]"
	}
	return writes, "access keys, SSO tokens or secrets"
}

// InstallHint implements provider.Installer.
func (p *Provider) InstallHint() string {
	return "install the AWS CLI: brew install awscli, or the AWS CLI v2 installer from https://aws.amazon.com/cli/"
}
