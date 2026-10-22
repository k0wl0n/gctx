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
	report := providerutil.CheckEach(context.Background(), accts, func(_ context.Context, acct config.Account) (bool, string) {
		return acct.Name == "a", "id-" + acct.Name
	})
	if !report.Credentialed["a"] || report.Credentialed["b"] || len(report.Credentialed) != 2 {
		t.Fatalf("credentialed %v", report.Credentialed)
	}
	if report.Identities["a"] != "id-a" || len(report.Warnings) != 0 {
		t.Fatalf("report %+v", report)
	}
}

func TestCheckEachWarnsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	report := providerutil.CheckEach(ctx, []config.Account{{Name: "a"}}, func(context.Context, config.Account) (bool, string) {
		cancel()
		return true, ""
	})
	if _, present := report.Credentialed["a"]; present || len(report.Warnings) != 1 || report.Warnings[0].Account != "a" {
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
