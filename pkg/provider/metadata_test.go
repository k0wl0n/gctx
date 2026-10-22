package provider_test

import (
	"slices"
	"testing"

	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/aws"
	"github.com/k0wl0n/gctx/pkg/provider/azure"
	"github.com/k0wl0n/gctx/pkg/provider/gcp"
)

func TestProvidersExposeMetadata(t *testing.T) {
	tests := []struct {
		provider provider.Provider
		badge    provider.Badge
		slot     string
		columns  map[string]string // header -> value for the sample account
		form     []string
		badInput string // rejected by the target field's Validate
	}{
		{
			provider: gcp.New(gcp.Options{}),
			badge:    provider.Badge{Text: "GCP", Style: provider.BadgeFilled, Color: "#6ea8fe"},
			slot:     "gcp",
			form:     []string{provider.FormTargetKey},
			badInput: "Bad_Project",
		},
		{
			provider: aws.New(aws.Options{}),
			badge:    provider.Badge{Text: "AWS", Style: provider.BadgeFilled, Color: "#ffa657"},
			slot:     "aws",
			columns:  map[string]string{"REGION": "eu-west-1"},
			form:     []string{"auth", provider.FormTargetKey, "sso_start_url", "sso_region", "region", "sso_account_id", "sso_role_name"},
			badInput: "bad profile",
		},
		{
			provider: azure.New(azure.Options{ConfigRoot: t.TempDir()}),
			badge:    provider.Badge{Text: "AZR", Style: provider.BadgeFilled, Color: "#56d4dd"},
			slot:     "azure",
			columns:  map[string]string{"TENANT": "tenant-1"},
			form:     []string{provider.FormTargetKey, "tenant"},
			badInput: "not-a-guid",
		},
	}
	view := provider.AccountView{Name: "work", Target: "t", Settings: map[string]string{"region": "eu-west-1", "tenant": "tenant-1"}}
	for _, tt := range tests {
		p := tt.provider
		t.Run(string(p.ID()), func(t *testing.T) {
			if p.Category() != provider.CategoryCloud {
				t.Errorf("category %q", p.Category())
			}
			if p.Badge() != tt.badge {
				t.Errorf("badge %+v, want %+v", p.Badge(), tt.badge)
			}
			if p.Slot() != tt.slot || p.Slot() != string(p.ID()) {
				t.Errorf("slot %q", p.Slot())
			}
			columns := p.Columns()
			if len(columns) != len(tt.columns) {
				t.Fatalf("columns %+v", columns)
			}
			for _, column := range columns {
				want, ok := tt.columns[column.Header]
				if !ok || column.Key == "" || column.Width <= 0 || column.Value(view) != want {
					t.Errorf("column %+v value %q", column, column.Value(view))
				}
			}
			var keys []string
			for _, field := range p.Form() {
				keys = append(keys, field.Key)
				if field.Label == "" || field.Secret {
					t.Errorf("field %+v", field)
				}
			}
			if !slices.Equal(keys, tt.form) {
				t.Errorf("form keys %v, want %v", keys, tt.form)
			}
			target := p.Form()[slices.Index(keys, provider.FormTargetKey)]
			if target.Validate == nil || target.Validate(tt.badInput) == nil {
				t.Errorf("target field does not reject %q", tt.badInput)
			}
		})
	}
}

func TestAWSFormAuthDefaultsToSSO(t *testing.T) {
	auth := aws.New(aws.Options{}).Form()[0]
	if auth.Key != "auth" || auth.Default != "sso" || !slices.Equal(auth.Options, []string{"sso", "profile"}) {
		t.Errorf("auth field %+v", auth)
	}
}
