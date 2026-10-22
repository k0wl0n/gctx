package providerutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/providerutil"
)

func TestCheckEachKeysResultsByName(t *testing.T) {
	accts := []config.Account{{Name: "a"}, {Name: "b"}}
	report := providerutil.CheckEach(context.Background(), accts, func(_ context.Context, acct config.Account) provider.AuthResult {
		return provider.AuthResult{Credentialed: acct.Name == "a", Identity: "id-" + acct.Name}
	})
	if !report.Results["a"].Credentialed || report.Results["b"].Credentialed || len(report.Results) != 2 {
		t.Fatalf("credentialed %v", report.Results)
	}
	if report.Results["a"].Identity != "id-a" || len(report.Warnings) != 0 {
		t.Fatalf("report %+v", report)
	}
}

func TestCheckEachWarnsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	report := providerutil.CheckEach(ctx, []config.Account{{Name: "a"}}, func(context.Context, config.Account) provider.AuthResult {
		cancel()
		return provider.AuthResult{Credentialed: true}
	})
	// A cut-short check is no verdict: it carries Err (UNVERIFIED), never Credentialed.
	if result := report.Results["a"]; result.Credentialed || result.Err == nil || len(report.Warnings) != 1 || report.Warnings[0].Account != "a" {
		t.Fatalf("report %+v", report)
	}
}

func TestResolveRunner(t *testing.T) {
	notFound := errors.New("missing")
	if _, err := providerutil.ResolveRunner(nil, provider.CLIStatus{}, notFound); !errors.Is(err, notFound) {
		t.Fatalf("err %v", err)
	}
	if runner, err := providerutil.ResolveRunner(nil, provider.CLIStatus{Bin: "/bin/sh", Found: true}, notFound); err != nil || runner == nil {
		t.Fatalf("runner %v err %v", runner, err)
	}
}
