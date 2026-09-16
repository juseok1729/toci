package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

// TestRenderPickerActionKindUsesSearchColors checks the requested styling:
// the action picker's border/title match the ":" resource search's own
// red, its cursor row is padded flush to the box's own width (like the
// search box's row highlight) instead of only as wide as the label text,
// and unselected items render in the search box's pink label color.
func TestRenderPickerActionKindUsesSearchColors(t *testing.T) {
	m := Model{}
	m.picker = newPicker(pickerAction, "action: my-instance", []pickerItem{
		{key: "start", label: "Start"},
		{key: "stop", label: "Stop"},
	})

	out := m.renderPicker()
	lines := strings.Split(out, "\n")

	borderColor := "38;2;252;100;100" // splashLogoStyle's #fc6464
	if !strings.Contains(lines[0], borderColor) {
		t.Errorf("border should use the search box's red, line 0 = %q", lines[0])
	}
	if !strings.Contains(lines[1], borderColor) || !strings.Contains(lines[1], "my-instance") {
		t.Errorf("title line should be styled in the search box's red, got %q", lines[1])
	}

	var cursorLine, itemLine string
	for _, l := range lines {
		if strings.Contains(l, "Start") {
			cursorLine = l
		}
		if strings.Contains(l, "Stop") {
			itemLine = l
		}
	}
	if !strings.Contains(cursorLine, "48;2;246;203;203") {
		t.Errorf("cursor row should use the search box's cursor background (#f6cbcb), got %q", cursorLine)
	}
	if !strings.Contains(itemLine, "38;2;246;203;203") || strings.Contains(itemLine, "48;2;246;203;203") {
		t.Errorf("unselected item should use the search box's pink foreground with no background, got %q", itemLine)
	}

	// The cursor row's own background should stretch flush to the box's
	// right border, not stop right after "Start" — same width as the
	// input line right below the title (which is textInputWidth-wide).
	stripped := ansi.Strip(cursorLine)
	inputStripped := ansi.Strip(lines[2])
	if len(stripped) != len(inputStripped) {
		t.Errorf("cursor row width = %d, want %d (matching the input line's width)", len(stripped), len(inputStripped))
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
