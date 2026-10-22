package gcp

import "github.com/k0wl0n/gctx/pkg/provider"

func (p *Provider) Category() provider.Category { return provider.CategoryCloud }

func (p *Provider) Badge() provider.Badge {
	return provider.Badge{Text: "GCP", Style: provider.BadgeFilled, Color: "#6ea8fe"}
}

func (p *Provider) Slot() string { return string(p.ID()) }

func (p *Provider) Columns() []provider.Column { return nil }

func (p *Provider) Form() []provider.FormField {
	return []provider.FormField{
		{Key: provider.FormTargetKey, Label: "Project ID", Validate: p.ValidateTarget},
	}
}

// Footer implements provider.FormFooter.
func (p *Provider) Footer(values map[string]string) (writes, never string) {
	configName := "<name>"
	if name := values[provider.FormNameKey]; name != "" {
		configName = ConfigNameFor(name)
	}
	return "config.json (name, project) · gcloud configuration " + configName, "passwords, tokens or keys"
}

// InstallHint implements provider.Installer.
func (p *Provider) InstallHint() string {
	return "install the Google Cloud SDK: https://cloud.google.com/sdk/docs/install"
}
