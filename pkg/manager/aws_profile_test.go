package manager_test

import (
	"context"
	"testing"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestAWSFlagLikeProfileRefusedAtCreateAndImport(t *testing.T) {
	t.Setenv("PATH", "")
	fx := newDiscoveryFixture(t, nil)
	if _, err := fx.manager.CreateAccount(context.Background(), manager.CreateRequest{Provider: provider.AWS, Name: "dbg", Target: "--debug"}); err == nil {
		t.Error("create accepted profile --debug")
	}
	result, err := fx.manager.ImportAccounts([]manager.DiscoveredAccount{{Provider: provider.AWS, Name: "dbg", Target: "--debug"}})
	if err != nil || len(result.Imported) != 0 || len(result.Skipped) != 1 {
		t.Errorf("import %+v err %v, want it skipped", result, err)
	}
}
