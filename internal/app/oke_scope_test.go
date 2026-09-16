package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"toci/internal/registry"
)

// TestSelectOkeFilterOpensOkeMenu checks OCI's own OKE cluster detail page
// layout: Enter on an "oke" row should float a 2-item menu (Node Pools,
// Add-ons), not the full ":" search across every resource kind, and
// remember the row as m.pendingRow for "Add-ons" (see confirmPicker).
func TestSelectOkeFilterOpensOkeMenu(t *testing.T) {
	m := &Model{resources: registry.All(nil), table: newTable(20)}
	row := registry.Row{ID: "oke1", Name: "my-cluster"}
	m.selectOkeFilter(row)

	if m.mode != modePicker || m.picker.kind != pickerResource {
		t.Fatalf("mode = %v, picker.kind = %v, want modePicker/pickerResource", m.mode, m.picker.kind)
	}
	if m.scope.OkeID != "oke1" || m.okeFilterName != "my-cluster" {
		t.Errorf("scope.OkeID = %q, okeFilterName = %q, want %q/%q", m.scope.OkeID, m.okeFilterName, "oke1", "my-cluster")
	}
	if m.pendingRow.ID != "oke1" {
		t.Errorf("pendingRow = %+v, want the OKE row itself", m.pendingRow)
	}

	got := make([]string, len(m.picker.filtered))
	for i, it := range m.picker.filtered {
		got[i] = it.key
	}
	want := []string{"oke-node-pool", "oke-addons"}
	if len(got) != len(want) {
		t.Fatalf("picker items = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("picker items = %v, want %v", got, want)
			break
		}
	}
}

// TestConfirmPickerNodePoolsSwitchesResource: picking "Node Pools" off the
// OKE menu should behave like any other pickerResource item — switch to
// the real "oke-node-pool" resource, keeping scope.OkeID.
func TestConfirmPickerNodePoolsSwitchesResource(t *testing.T) {
	m := &Model{resources: registry.All(nil), table: newTable(20)}
	m.selectOkeFilter(registry.Row{ID: "oke1", Name: "my-cluster"})

	mi, _ := m.confirmPicker()
	m2 := mi.(Model)

	if m2.mode != modeTable || m2.current().Key() != "oke-node-pool" {
		t.Errorf("mode = %v, resource = %v, want modeTable/oke-node-pool", m2.mode, m2.current().Key())
	}
	if m2.scope.OkeID != "oke1" {
		t.Errorf("scope.OkeID = %q, want %q (should stay set)", m2.scope.OkeID, "oke1")
	}
}

// TestConfirmPickerAddonsFetchesStatus: picking "Add-ons" should not match
// any registered resource (it isn't one) — it must be special-cased to
// fetch add-on status for m.pendingRow instead of silently doing nothing.
func TestConfirmPickerAddonsFetchesStatus(t *testing.T) {
	m := &Model{resources: registry.All(nil), table: newTable(20)}
	row := registry.Row{ID: "oke1", Name: "my-cluster"}
	m.selectOkeFilter(row)
	m.picker.cursor = 1 // "Add-ons" is the second item

	mi, cmd := m.confirmPicker()
	m2 := mi.(Model)

	if m2.mode != modeTable {
		t.Errorf("mode = %v, want modeTable", m2.mode)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil command to fetch add-on status")
	}
}

// TestSwitchResourceRedirectsWhenOkeIDMissing mirrors
// TestSwitchResourceRedirectsWhenDrgIDRequiredButMissing: reaching Node
// Pools some other way (Tab, ":") without a cluster picked first would mix
// every node pool in the compartment together with no column showing
// which cluster each is in — see okeIDRequiredResourceKeys' doc.
func TestSwitchResourceRedirectsWhenOkeIDMissing(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}

	okeIdx, nodePoolIdx := -1, -1
	for i, r := range m.resources {
		switch r.Key() {
		case "oke":
			okeIdx = i
		case "oke-node-pool":
			nodePoolIdx = i
		}
	}
	if okeIdx < 0 || nodePoolIdx < 0 {
		t.Fatal("expected both \"oke\" and \"oke-node-pool\" to be registered resources")
	}

	m.switchResource(nodePoolIdx)

	if m.resIdx != okeIdx {
		t.Errorf("resIdx = %d (%s), want %d (oke) — should redirect when scope.OkeID is empty", m.resIdx, m.resources[m.resIdx].Key(), okeIdx)
	}
	if m.statusMsg == "" {
		t.Error("expected a status message explaining why it redirected")
	}
}

func TestSwitchResourceLoadsNodePoolsWhenOkeIDSet(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}
	m.scope.OkeID = "ocid1.cluster.oc1..aaa"

	nodePoolIdx := -1
	for i, r := range m.resources {
		if r.Key() == "oke-node-pool" {
			nodePoolIdx = i
		}
	}
	if nodePoolIdx < 0 {
		t.Fatal("expected \"oke-node-pool\" to be a registered resource")
	}

	m.switchResource(nodePoolIdx)

	if m.resIdx != nodePoolIdx {
		t.Errorf("resIdx = %d (%s), want %d (oke-node-pool) — should not redirect once scope.OkeID is set", m.resIdx, m.resources[m.resIdx].Key(), nodePoolIdx)
	}
}

// TestIKeyOnOkeRowMatchesEnter: "i" is documented as "same as Enter" on a
// VCN/DRG row — it should work the same way on an "oke" row too.
func TestIKeyOnOkeRowMatchesEnter(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20)}
	for i, r := range m.resources {
		if r.Key() == "oke" {
			m.resIdx = i
			break
		}
	}
	m.displayRows = []registry.Row{{ID: "oke1", Name: "my-cluster"}}

	mi, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m2 := mi.(Model)

	if m2.mode != modePicker || m2.picker.kind != pickerResource {
		t.Fatalf(`"i" on an oke row: mode = %v, picker.kind = %v, want modePicker/pickerResource`, m2.mode, m2.picker.kind)
	}
	if m2.scope.OkeID != "oke1" {
		t.Errorf("scope.OkeID = %q, want %q", m2.scope.OkeID, "oke1")
	}
}
