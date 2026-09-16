package app

import (
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"toci/internal/registry"
)

var (
	backspaceMsg = tea.KeyPressMsg{Code: tea.KeyBackspace}
	escMsg       = tea.KeyPressMsg{Code: tea.KeyEsc}
)

// TestEscInTableDoesNotExitVcnFilterAnymore locks in the requested split:
// esc's job is closing a floating window/view (the "f" search, "M"
// resource map, "v" rules view, ...) — modeTable itself has none of those
// open, so esc should be a no-op there. Clearing scope/filters and
// cancelling a fetch is "back to the previous screen" now — backspace's
// job (see TestBackspaceGoesBackInTableWhenNoTextInput below).
func TestEscInTableDoesNotExitVcnFilterAnymore(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20)}
	m.scope.VcnID = "vcn1"
	m.vcnFilterName = "my-vcn"
	m.filterQuery = "abc"

	mi, _ := m.Update(escMsg)
	m2 := mi.(Model)
	if m2.scope.VcnID != "vcn1" || m2.vcnFilterName != "my-vcn" {
		t.Errorf("esc in modeTable should no longer exit the VCN filter, got VcnID=%q vcnFilterName=%q", m2.scope.VcnID, m2.vcnFilterName)
	}
	if m2.filterQuery != "abc" {
		t.Errorf("esc in modeTable should no longer clear the filter query, got %q", m2.filterQuery)
	}
}

// TestBackspaceGoesBackInTableWhenNoTextInput: modeTable and modeDetail
// have no active text input, so backspace should act on the very first
// press — clearing scope/filters (modeTable) or closing an overlay
// (modeDetail), the "go back" role esc no longer plays in modeTable.
func TestBackspaceGoesBackInTableWhenNoTextInput(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20)}
	m.scope.VcnID = "vcn1"
	m.vcnFilterName = "my-vcn"

	mi, _ := m.Update(backspaceMsg)
	m2 := mi.(Model)
	if m2.scope.VcnID != "" || m2.vcnFilterName != "" {
		t.Errorf("backspace in modeTable should exit the VCN filter, got VcnID=%q vcnFilterName=%q", m2.scope.VcnID, m2.vcnFilterName)
	}

	detail := Model{mode: modeDetail, resources: registry.All(nil), detail: viewport.New(), rulesOverlay: true}
	mi, _ = detail.Update(backspaceMsg)
	d2 := mi.(Model)
	if d2.mode != modeTable || d2.rulesOverlay {
		t.Errorf("backspace in modeDetail should close the rules overlay like esc/q/v, got mode=%v rulesOverlay=%v", d2.mode, d2.rulesOverlay)
	}
}

// TestBackspaceStepsBackThroughResourceHistory locks in the requested
// behavior: VCN -> Subnet -> DB System, then backspace should land back on
// Subnet, and backspace again on VCN — one step per press, not a single
// jump straight out of the VCN scope the way exitVcn does.
func TestBackspaceStepsBackThroughResourceHistory(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20)}

	var vcnIdx, subnetIdx, dbSystemIdx = -1, -1, -1
	for i, r := range m.resources {
		switch r.Key() {
		case "vcn":
			vcnIdx = i
		case "subnet":
			subnetIdx = i
		case "db-system":
			dbSystemIdx = i
		}
	}
	if vcnIdx < 0 || subnetIdx < 0 || dbSystemIdx < 0 {
		t.Fatal("expected \"vcn\", \"subnet\", and \"db-system\" to be registered resources")
	}

	m.switchResource(vcnIdx) // the very first switch — shouldn't push anything (nothing real to go back to yet)
	m.scope.VcnID = "vcn1"
	m.vcnFilterName = "my-vcn"
	m.switchResource(subnetIdx)
	m.switchResource(dbSystemIdx)
	if m.resIdx != dbSystemIdx {
		t.Fatalf("resIdx = %d, want dbSystemIdx = %d", m.resIdx, dbSystemIdx)
	}

	mi, _ := m.Update(backspaceMsg)
	m2 := mi.(Model)
	if m2.resIdx != subnetIdx {
		t.Errorf("first backspace: resIdx = %d (%s), want subnetIdx = %d", m2.resIdx, m2.resources[m2.resIdx].Key(), subnetIdx)
	}

	mi, _ = m2.Update(backspaceMsg)
	m3 := mi.(Model)
	if m3.resIdx != vcnIdx {
		t.Errorf("second backspace: resIdx = %d (%s), want vcnIdx = %d", m3.resIdx, m3.resources[m3.resIdx].Key(), vcnIdx)
	}
}

// TestBackspaceNeverClosesPicker: modePicker's backspace is pure text
// editing — esc is the only key that closes it, even once the query is
// empty (closing on an empty backspace press was indistinguishable from
// esc and surprised users expecting backspace to never dismiss a window).
func TestBackspaceNeverClosesPicker(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}
	m.openResourceSearch()

	m.picker.input.SetValue("vcn")
	mi, _ := m.Update(backspaceMsg)
	m2 := mi.(Model)
	if m2.mode != modePicker {
		t.Fatalf("backspace with a non-empty query should edit the text, not close the picker; mode = %v", m2.mode)
	}
	if got := m2.picker.input.Value(); got != "vc" {
		t.Errorf("backspace should have deleted the last character, input = %q, want %q", got, "vc")
	}

	m2.picker.input.SetValue("")
	mi, _ = m2.Update(backspaceMsg)
	m3 := mi.(Model)
	if m3.mode != modePicker {
		t.Errorf("backspace with an empty query should not close the picker, mode = %v, want modePicker", m3.mode)
	}
}

// TestBackspaceNeverClearsFilter is modePicker's test above, mirrored for
// the "/" filter input.
func TestBackspaceNeverClearsFilter(t *testing.T) {
	m := Model{mode: modeFilter, resources: registry.All(nil), table: newTable(20), filterInput: textinput.New()}
	m.filterInput.SetValue("abc")
	m.filterInput.Focus() // textinput.Update ignores keys while blurred
	m.filterBak = ""

	mi, _ := m.Update(backspaceMsg)
	m2 := mi.(Model)
	if m2.mode != modeFilter {
		t.Fatalf("backspace with non-empty filter text should edit it, not close the filter; mode = %v", m2.mode)
	}
	if got := m2.filterInput.Value(); got != "ab" {
		t.Errorf("backspace should have deleted the last character, input = %q, want %q", got, "ab")
	}

	m2.filterInput.SetValue("")
	mi, _ = m2.Update(backspaceMsg)
	m3 := mi.(Model)
	if m3.mode != modeFilter {
		t.Errorf("backspace with an empty filter should not close it, mode = %v, want modeFilter", m3.mode)
	}
}
