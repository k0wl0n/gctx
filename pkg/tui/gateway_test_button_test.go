package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

const testGatewayID provider.ID = "testgw"

func gatewayInfo(canTest bool) manager.ProviderInfo {
	return manager.ProviderInfo{
		ID: testGatewayID, Label: "TestGw", Category: provider.CategoryAIGateway,
		Badge: provider.Badge{Text: "TGW", Color: "#ff66aa"},
		CLI:   provider.CLIStatus{Found: true, Name: "testgw"}, CanTest: canTest,
		Form: []provider.FormField{{Key: "base_url", Label: "Base URL"}},
	}
}

func gatewayForm(t *testing.T, svc *fakeService, canTest bool) (Model, *accountForm) {
	t.Helper()
	svc.overview.Providers = append(svc.overview.Providers, gatewayInfo(canTest))
	m := newTestModel(t, svc, 120, 40)
	m.modal = newCreateForm(svc.overview, testGatewayID)
	return m, m.modal.(*accountForm)
}

func focusItem(f *accountForm, kind formItemKind) {
	for i, item := range f.items() {
		if item.kind == kind {
			f.pos = i
		}
	}
}

func TestGatewayFormOffersTestAgain(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	svc.testRes = provider.ConnectionResult{Detail: "budget $0.00 / $10.00"}
	m, f := gatewayForm(t, svc, true)
	if !strings.Contains(screen(m), "Test again") {
		t.Fatalf("no Test again button:\n%s", screen(m))
	}
	focusItem(f, itemTest)
	m = press(t, m, "enter")
	if !slices.Contains(svc.Calls(), "TestConnection "+string(testGatewayID)) {
		t.Fatalf("check not run: %v", svc.Calls())
	}
	if out := screen(m); !strings.Contains(out, "✓ budget $0.00 / $10.00") {
		t.Fatalf("no success line:\n%s", out)
	}
	if m.modal == nil {
		t.Fatal("modal closed after test")
	}
}

func TestGatewayTestAgainShowsFailure(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	svc.testErr = errors.New("401 unauthorized")
	m, f := gatewayForm(t, svc, true)
	focusItem(f, itemTest)
	m = press(t, m, "enter")
	if out := screen(m); !strings.Contains(out, "✗ 401 unauthorized") {
		t.Fatalf("no failure line:\n%s", out)
	}
}

func TestTestAgainOnlyForProvidersThatCanTest(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	m, _ := gatewayForm(t, svc, false)
	if strings.Contains(screen(m), "Test again") {
		t.Fatalf("button shown for a provider without the hook:\n%s", screen(m))
	}
	gcp := newTestModel(t, newFakeService(sampleOverview("alice", "alice")), 120, 40)
	gcp, _ = gcp.openCreate()
	if strings.Contains(screen(gcp), "Test again") {
		t.Fatal("button shown on the GCP form")
	}
}
