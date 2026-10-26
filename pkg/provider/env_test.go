package provider_test

import (
	"slices"
	"testing"

	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestMergeEnvAppliesSetsAndUnsets(t *testing.T) {
	base := []string{"KEEP=1", "DROP=old", "SET=old"}
	got := provider.MergeEnv(base, []string{"SET=new", "DROP", "ADDED=x"})
	want := []string{"KEEP=1", "SET=old", "SET=new", "ADDED=x"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if !slices.Equal(base, []string{"KEEP=1", "DROP=old", "SET=old"}) {
		t.Fatalf("base modified: %v", base)
	}
}
