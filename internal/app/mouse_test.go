package app

import (
	"testing"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"toci/internal/registry"
)

func TestMouseClickRow(t *testing.T) {
	// Cursor is row 5, drawn on screen line 11, table body is 20 lines tall.
	const cursorY, cursorRow, bodyHeight = 11, 5, 20

	cases := []struct {
		name    string
		clickY  int
		wantRow int
		wantOK  bool
	}{
		{"click on cursor's own row", cursorY, cursorRow, true},
		{"click two rows below cursor", cursorY + 2, cursorRow + 2, true},
		{"click three rows above cursor", cursorY - 3, cursorRow - 3, true},
		{"click above the table (header lines)", mouseBodyTop - 1, 0, false},
		{"click past the table body", mouseBodyTop + bodyHeight, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row, ok := mouseClickRow(c.clickY, cursorY, cursorRow, bodyHeight)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && row != c.wantRow {
				t.Errorf("row = %d, want %d", row, c.wantRow)
			}
		})
	}
}

func TestUpdateMouseWheelMovesCursor(t *testing.T) {
	m := Model{table: newTable(20), mode: modeTable}
	m.table.SetColumns([]table.Column{{Title: "NAME", Width: 5}})
	m.table.SetRows([]table.Row{{"a"}, {"b"}, {"c"}})

	mm, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m2 := mm.(Model)
	if got, want := m2.table.Cursor(), 1; got != want {
		t.Errorf("cursor after wheel-down = %d, want %d", got, want)
	}

	mm2, _ := m2.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m3 := mm2.(Model)
	if got, want := m3.table.Cursor(), 0; got != want {
		t.Errorf("cursor after wheel-up = %d, want %d", got, want)
	}
}

func TestPickerItemAt(t *testing.T) {
	// 10 lines tall, box top-left at (2, 5) — enough room to fit the
	// border + either layout's header lines + a few items below them.
	box := "0123456789\n0123456789\n0123456789\n0123456789\n0123456789\n0123456789\n0123456789\n0123456789\n0123456789\n0123456789"
	const boxX, boxY = 2, 5

	cases := []struct {
		name     string
		x, y     int
		itemsTop int
		wantIdx  int
		wantOK   bool
	}{
		{"first item, regular picker layout", boxX, boxY + 1 + pickerRegularItemsTop, pickerRegularItemsTop, 0, true},
		{"second item, resource-search layout", boxX, boxY + 1 + pickerResourceItemsTop + 1, pickerResourceItemsTop, 1, true},
		{"click on the border/title area", boxX, boxY, pickerRegularItemsTop, 0, false},
		{"click left of the box", boxX - 1, boxY + 1 + pickerRegularItemsTop, pickerRegularItemsTop, 0, false},
		{"click past the box's bottom edge", boxX, boxY + 20, pickerRegularItemsTop, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idx, ok := pickerItemAt(c.x, c.y, boxX, boxY, c.itemsTop, box)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && idx != c.wantIdx {
				t.Errorf("idx = %d, want %d", idx, c.wantIdx)
			}
		})
	}
}

func TestUpdatePickerMouseClickSelectsAndConfirms(t *testing.T) {
	m := Model{
		resources: registry.All(nil),
		width:     100,
		height:    30,
		mode:      modePicker,
		table:     newTable(20),
		picker: newPicker(pickerRegion, "region", []pickerItem{
			{key: "us-ashburn-1", label: "us-ashburn-1"},
			{key: "us-phoenix-1", label: "us-phoenix-1"},
		}),
	}

	// The regular (non-resource-search) picker box sits at the fixed
	// (2, 6) screen offset — see updatePickerMouse — with its own top
	// border on the first line, so item i lands at boxY + 1 + itemsTop + i.
	clickY := 6 + 1 + pickerRegularItemsTop + 1 // second item ("us-phoenix-1")
	mm, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: clickY, Button: tea.MouseLeft})
	m2 := mm.(Model)

	if m2.mode != modeTable {
		t.Fatalf("mode after click = %v, want modeTable (click should confirm like enter)", m2.mode)
	}
	if m2.scope.Region != "us-phoenix-1" {
		t.Errorf("scope.Region = %q, want us-phoenix-1", m2.scope.Region)
	}
	if cmd == nil {
		t.Error("expected a load command after the click confirmed a region")
	}
}

// TestUpdatePickerMouseClickOnResourceSearchSelectsWithoutConfirming
// guards a reported UX bug: clicking a row in the "f" resource-search tree
// used to switch resources immediately, same as the regular pickers above —
// but the tree's category headers sit between real resources, making a
// stray click easy to land on and instantly commit. A click there should
// only move the cursor; Enter confirms, like keyboard navigation already
// does.
func TestUpdatePickerMouseClickOnResourceSearchSelectsWithoutConfirming(t *testing.T) {
	m := Model{
		resources: registry.All(nil),
		width:     120,
		height:    40,
		mode:      modePicker,
		table:     newTable(20),
	}
	m.openResourceSearch()

	box := m.renderResourceSearch()
	boxWidth, boxLines := overlayBoxDims(box)
	boxX := (m.width - boxWidth) / 2
	boxY := (m.height - len(boxLines)) / 3

	// Item index 1 is the first real resource — index 0 is always a
	// category header (see resourcePickerItems).
	clickY := boxY + 1 + pickerResourceItemsTop + 1
	mm, cmd := m.Update(tea.MouseClickMsg{X: boxX + 4, Y: clickY, Button: tea.MouseLeft})
	m2 := mm.(Model)

	if m2.mode != modePicker {
		t.Fatalf("mode after click = %v, want modePicker (a click should only select, not confirm)", m2.mode)
	}
	if m2.picker.cursor != 1 {
		t.Errorf("picker.cursor = %d, want 1 (the clicked row)", m2.picker.cursor)
	}
	if cmd != nil {
		t.Error("expected no command — a click on the resource search should not switch resources yet")
	}
}
