package azure

import "github.com/k0wl0n/gctx/pkg/provider"

func (p *Provider) Category() provider.Category { return provider.CategoryCloud }

func (p *Provider) Badge() provider.Badge {
	return provider.Badge{Text: "AZR", Style: provider.BadgeFilled, Color: "#56d4dd"}
}

func (p *Provider) Slot() string { return string(p.ID()) }

// EnvBased reports that activation only prints an env line (provider.EnvBased).
func (p *Provider) EnvBased() bool { return true }

func (p *Provider) Columns() []provider.Column {
	return []provider.Column{
		{Key: "tenant", Header: "TENANT", Width: 36, Value: func(v provider.AccountView) string { return v.Settings["tenant"] }},
	}
}

func (p *Provider) Form() []provider.FormField {
	return []provider.FormField{
		{Key: provider.FormTargetKey, Label: "Subscription ID", Validate: p.ValidateTarget},
		{Key: "tenant", Label: "Tenant", Help: "optional"},
	}
}
