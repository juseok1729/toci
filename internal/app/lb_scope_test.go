package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"toci/internal/registry"
)

// TestSelectLbFilterOpensLbMenu checks OCI's own load balancer detail page
// layout: "i"/Enter on a "lb" row should float a flat picker over exactly
// lbPickerResourceKeys, in that order — Listeners, Backend Sets, Routing
// Policies, Rule Sets, Path Route Sets, Certificates, Hostnames.
func TestSelectLbFilterOpensLbMenu(t *testing.T) {
	m := &Model{resources: registry.All(nil), table: newTable(20)}
	m.selectLbFilter("lb1", "my-lb", "")

	if m.mode != modePicker || m.picker.kind != pickerResource {
		t.Fatalf("mode = %v, picker.kind = %v, want modePicker/pickerResource", m.mode, m.picker.kind)
	}
	if m.scope.LbID != "lb1" || m.lbFilterName != "my-lb" {
		t.Errorf("scope.LbID = %q, lbFilterName = %q, want %q/%q", m.scope.LbID, m.lbFilterName, "lb1", "my-lb")
	}

	got := make([]string, len(m.picker.filtered))
	for i, it := range m.picker.filtered {
		got[i] = it.key
	}
	want := lbPickerResourceKeys
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

// TestSelectLbFilterUsesRowCompartmentID mirrors selectVcnFilter's own
// subtree-fan-out doc: a row's own CompartmentID (set only during a "C"
// subtree fan-out) must win over whatever compartment is currently in
// scope, since the LB may live in a different sub-compartment than the
// fan-out's base.
func TestSelectLbFilterUsesRowCompartmentID(t *testing.T) {
	m := &Model{resources: registry.All(nil), table: newTable(20)}
	m.scope.CompartmentID = "base-compartment"
	m.selectLbFilter("lb1", "my-lb", "sub-compartment")

	if m.scope.CompartmentID != "sub-compartment" {
		t.Errorf("scope.CompartmentID = %q, want %q (row's own compartment)", m.scope.CompartmentID, "sub-compartment")
	}
}

// TestSwitchResourceRedirectsWhenLbIDRequiredButMissing mirrors
// TestSwitchResourceRedirectsWhenDrgIDRequiredButMissing: every LB-scoped
// resource reads off one GetLoadBalancer call (registry/lb_scoped.go),
// which needs an ID to call at all — reaching one some other way (Tab,
// ":") without a load balancer picked first must redirect back to the "lb"
// list instead of erroring.
func TestSwitchResourceRedirectsWhenLbIDRequiredButMissing(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}

	lbIdx, listenerIdx := -1, -1
	for i, r := range m.resources {
		switch r.Key() {
		case "lb":
			lbIdx = i
		case "lb-listener":
			listenerIdx = i
		}
	}
	if lbIdx < 0 || listenerIdx < 0 {
		t.Fatal("expected both \"lb\" and \"lb-listener\" to be registered resources")
	}

	m.switchResource(listenerIdx)

	if m.resIdx != lbIdx {
		t.Errorf("resIdx = %d (%s), want %d (lb) — should redirect when scope.LbID is empty", m.resIdx, m.resources[m.resIdx].Key(), lbIdx)
	}
	if m.statusMsg == "" {
		t.Error("expected a status message explaining why it redirected")
	}
}

func TestSwitchResourceLoadsLbListenerWhenLbIDSet(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}
	m.scope.LbID = "ocid1.loadbalancer.oc1..aaa"

	listenerIdx := -1
	for i, r := range m.resources {
		if r.Key() == "lb-listener" {
			listenerIdx = i
		}
	}
	if listenerIdx < 0 {
		t.Fatal("expected \"lb-listener\" to be a registered resource")
	}

	m.switchResource(listenerIdx)

	if m.resIdx != listenerIdx {
		t.Errorf("resIdx = %d (%s), want %d (lb-listener) — should not redirect once scope.LbID is set", m.resIdx, m.resources[m.resIdx].Key(), listenerIdx)
	}
}

// TestSwitchResourceClearsLbFilterWhenLeaving mirrors the VCN/DRG/OKE
// filter-clearing behavior: picking a non-LB-scoped resource must drop
// scope.LbID/lbFilterName, the same as switching away from a VCN drops
// scope.VcnID.
func TestSwitchResourceClearsLbFilterWhenLeaving(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}
	m.scope.LbID = "lb1"
	m.lbFilterName = "my-lb"

	instanceIdx := -1
	for i, r := range m.resources {
		if r.Key() == "instance" {
			instanceIdx = i
		}
	}
	if instanceIdx < 0 {
		t.Fatal("expected \"instance\" to be a registered resource")
	}

	m.switchResource(instanceIdx)

	if m.scope.LbID != "" || m.lbFilterName != "" {
		t.Errorf("scope.LbID = %q, lbFilterName = %q, want both cleared after switching to a non-LB-scoped resource", m.scope.LbID, m.lbFilterName)
	}
}

// TestIKeyOnLbRowMatchesEnter: "i" is documented as "same as Enter" on a
// VCN/DRG/OKE row — it should work the same way on a "lb" row too.
func TestIKeyOnLbRowMatchesEnter(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20)}
	for i, r := range m.resources {
		if r.Key() == "lb" {
			m.resIdx = i
			break
		}
	}
	m.displayRows = []registry.Row{{ID: "lb1", Name: "my-lb"}}

	mi, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m2 := mi.(Model)

	if m2.mode != modePicker || m2.picker.kind != pickerResource {
		t.Fatalf(`"i" on a lb row: mode = %v, picker.kind = %v, want modePicker/pickerResource`, m2.mode, m2.picker.kind)
	}
	if m2.scope.LbID != "lb1" {
		t.Errorf("scope.LbID = %q, want %q", m2.scope.LbID, "lb1")
	}
}

// TestEnterOnLbRowMatchesI is TestIKeyOnLbRowMatchesEnter's Enter-key
// counterpart.
func TestEnterOnLbRowMatchesI(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20)}
	for i, r := range m.resources {
		if r.Key() == "lb" {
			m.resIdx = i
			break
		}
	}
	m.displayRows = []registry.Row{{ID: "lb1", Name: "my-lb"}}

	mi, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := mi.(Model)

	if m2.mode != modePicker || m2.picker.kind != pickerResource {
		t.Fatalf("Enter on a lb row: mode = %v, picker.kind = %v, want modePicker/pickerResource", m2.mode, m2.picker.kind)
	}
	if m2.scope.LbID != "lb1" {
		t.Errorf("scope.LbID = %q, want %q", m2.scope.LbID, "lb1")
	}
}

// TestBackspaceExitsLbFilterWhenNoHistory mirrors
// TestBackspaceGoesBackInTableWhenNoTextInput's VCN case: with no history
// to step back through, backspace should fall back to exitLb's direct
// jump rather than getting stuck scoped to the load balancer forever.
func TestBackspaceExitsLbFilterWhenNoHistory(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20)}
	m.scope.LbID = "lb1"
	m.lbFilterName = "my-lb"

	mi, _ := m.Update(backspaceMsg)
	m2 := mi.(Model)
	if m2.scope.LbID != "" || m2.lbFilterName != "" {
		t.Errorf("backspace in modeTable should exit the LB filter, got LbID=%q lbFilterName=%q", m2.scope.LbID, m2.lbFilterName)
	}
}
