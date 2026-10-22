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
