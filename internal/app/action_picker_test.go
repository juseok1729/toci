package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"toci/internal/registry"
)

var enterMsg = tea.KeyPressMsg{Code: tea.KeyEnter}

// instanceModelForActionTest builds a Model sitting on the Instance
// resource with one selectable row — the setup every test below needs to
// exercise Enter's action-picker path (updateTable's default case,
// openActionPicker).
func instanceModelForActionTest(writeEnabled bool) Model {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20), writeEnabled: writeEnabled, width: 160, height: 45}
	for i, r := range m.resources {
		if r.Key() == "instance" {
			m.resIdx = i
			break
		}
	}
	m.displayRows = []registry.Row{{ID: "i1", Name: "my-instance"}}
	return m
}

func TestEnterOnInstanceRowFloatsActionPickerOverTheTable(t *testing.T) {
	m := instanceModelForActionTest(true)

	mi, _ := m.Update(enterMsg)
	m2 := mi.(Model)

	if m2.mode != modePicker || m2.picker.kind != pickerAction {
		t.Fatalf("mode = %v, picker.kind = %v, want modePicker/pickerAction", m2.mode, m2.picker.kind)
	}
	if m2.pendingRow.Name != "my-instance" {
		t.Errorf("pendingRow.Name = %q, want %q", m2.pendingRow.Name, "my-instance")
	}

	// It should float over the table (like the resource-search/compartment
	// pickers), not replace the page — the table header stays visible
	// behind it.
	out := m2.viewContent()
	if !strings.Contains(out, "Profile:") {
		t.Errorf("expected the table header to still render behind the floating action picker, got:\n%s", out)
	}
}

// TestEscOnActionPickerClosesOnlyThatPickerNotAllTheWayToSplash reproduces
// a reported bug: opening the action picker left m.pickerReturnMode at
// whatever it was last set to by some earlier, unrelated picker (e.g.
// modeSplash, from the home screen's ":" search) — openActionPicker never
// set its own, so Esc closed all the way back to that stale target instead
// of just this floating action picker.
func TestEscOnActionPickerClosesOnlyThatPickerNotAllTheWayToSplash(t *testing.T) {
	m := instanceModelForActionTest(true)
	m.pickerReturnMode = modeSplash // stale, left over from an earlier, unrelated picker

	mi, _ := m.Update(enterMsg)
	m2 := mi.(Model)
	if m2.mode != modePicker || m2.picker.kind != pickerAction {
		t.Fatalf("mode = %v, picker.kind = %v, want modePicker/pickerAction", m2.mode, m2.picker.kind)
	}

	mi, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m3 := mi.(Model)
	if m3.mode != modeTable {
		t.Errorf("mode after Esc = %v, want modeTable (should close only the action picker, not jump to a stale pickerReturnMode)", m3.mode)
	}
}

func TestEnterOnInstanceRowInReadonlyModeShowsStatusInsteadOfOpeningPicker(t *testing.T) {
	m := instanceModelForActionTest(false)

	mi, _ := m.Update(enterMsg)
	m2 := mi.(Model)

	if m2.mode != modeTable {
		t.Errorf("mode = %v, want modeTable (readonly: action picker should not open)", m2.mode)
	}
	if m2.statusMsg == "" {
		t.Error("expected a readonly status message")
	}
}

func TestEnterOnNonActionableRowInReadonlyModeStaysSilent(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20), writeEnabled: false}
	for i, r := range m.resources {
		if r.Key() == "subnet" {
			m.resIdx = i
			break
		}
	}
	m.displayRows = []registry.Row{{ID: "s1", Name: "my-subnet"}}

	mi, _ := m.Update(enterMsg)
	m2 := mi.(Model)

	if m2.mode != modeTable {
		t.Errorf("mode = %v, want modeTable", m2.mode)
	}
	// A non-actionable resource never had actions to begin with, so Enter
	// here must stay a true no-op — not the readonly-mode message, which
	// would wrongly imply this row would do something in --write mode.
	if m2.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty (non-actionable row, no misleading readonly message)", m2.statusMsg)
	}
}
