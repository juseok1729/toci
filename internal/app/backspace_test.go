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

// TestBackspaceOnlyClosesPickerWhenInputEmpty: modePicker's backspace must
// stay ordinary text-editing while there's a query typed — only closing
// (like esc) once there's nothing left to delete.
func TestBackspaceOnlyClosesPickerWhenInputEmpty(t *testing.T) {
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
	if m3.mode != m2.pickerReturnMode {
		t.Errorf("backspace with an empty query should close the picker like esc, mode = %v, want %v", m3.mode, m2.pickerReturnMode)
	}
}

// TestBackspaceOnlyClearsFilterWhenInputEmpty is modePicker's test above,
// mirrored for the "/" filter input.
func TestBackspaceOnlyClearsFilterWhenInputEmpty(t *testing.T) {
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
	if m3.mode != modeTable {
		t.Errorf("backspace with an empty filter should close it like esc, mode = %v, want modeTable", m3.mode)
	}
}
