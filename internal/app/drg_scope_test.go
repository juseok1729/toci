package app

import (
	"testing"

	"toci/internal/registry"
)

// Without this redirect, switching straight to DRG Route Table/Distribution
// (via Tab or "f") with no DRG picked yet hit the OCI API with an empty
// DrgId and came back as a raw 404 — see drgIDRequiredResourceKeys' doc.
func TestSwitchResourceRedirectsWhenDrgIDRequiredButMissing(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}

	drgIdx, routeTableIdx := -1, -1
	for i, r := range m.resources {
		switch r.Key() {
		case "drg":
			drgIdx = i
		case "drg-route-table":
			routeTableIdx = i
		}
	}
	if drgIdx < 0 || routeTableIdx < 0 {
		t.Fatal("expected both \"drg\" and \"drg-route-table\" to be registered resources")
	}

	m.switchResource(routeTableIdx)

	if m.resIdx != drgIdx {
		t.Errorf("resIdx = %d (%s), want %d (drg) — should redirect when scope.DrgID is empty", m.resIdx, m.resources[m.resIdx].Key(), drgIdx)
	}
	if m.statusMsg == "" {
		t.Error("expected a status message explaining why it redirected")
	}
}

func TestSwitchResourceLoadsDrgRouteTableWhenDrgIDSet(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}
	m.scope.DrgID = "ocid1.drg.oc1..aaa"

	routeTableIdx := -1
	for i, r := range m.resources {
		if r.Key() == "drg-route-table" {
			routeTableIdx = i
		}
	}
	if routeTableIdx < 0 {
		t.Fatal("expected \"drg-route-table\" to be a registered resource")
	}

	m.switchResource(routeTableIdx)

	if m.resIdx != routeTableIdx {
		t.Errorf("resIdx = %d (%s), want %d (drg-route-table) — should not redirect once scope.DrgID is set", m.resIdx, m.resources[m.resIdx].Key(), routeTableIdx)
	}
}
