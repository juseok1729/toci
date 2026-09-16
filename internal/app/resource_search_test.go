package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"toci/internal/registry"
)

// TestOnlyColonOpensResourceSearchInTable locks in the requested change:
// "f" used to be a shortcut for ":" here, which collided with typing an
// actual filename/label starting with "f" into other inputs elsewhere —
// ":" alone opens the search now.
func TestOnlyColonOpensResourceSearchInTable(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20)}

	mi, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if mi.(Model).mode == modePicker {
		t.Error(`"f" should no longer open the resource search`)
	}

	mi, _ = m.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	m2 := mi.(Model)
	if m2.mode != modePicker || m2.picker.kind != pickerResource {
		t.Errorf(`":" should open the resource search, got mode = %v, picker.kind = %v`, m2.mode, m2.picker.kind)
	}
}

var spaceMsg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}

// TestDoubleSpaceOpensResourceSearch: pressing space opens the shortcuts
// popup; pressing space again while it's still open (a double-tap) is a
// fast path straight into the resource search, the same destination as
// ":", just discoverable without already knowing that key.
func TestDoubleSpaceOpensResourceSearch(t *testing.T) {
	m := Model{mode: modeTable, resources: registry.All(nil), table: newTable(20)}

	mi, _ := m.Update(spaceMsg)
	m2 := mi.(Model)
	if !m2.showHelp {
		t.Fatalf("first space press should open the shortcuts popup, showHelp = %v", m2.showHelp)
	}

	mi, _ = m2.Update(spaceMsg)
	m3 := mi.(Model)
	if m3.showHelp {
		t.Error("second space press should close the shortcuts popup, not leave it open")
	}
	if m3.mode != modePicker || m3.picker.kind != pickerResource {
		t.Errorf("second space press should open the resource search, got mode = %v, picker.kind = %v", m3.mode, m3.picker.kind)
	}
}

func TestRenderResourceSearch(t *testing.T) {
	m := Model{
		resources: registry.All(nil),
		width:     120,
		height:    40,
	}
	m.openResourceSearch()

	for _, width := range []int{120, 60, 50, 20, 0} {
		m.width = width
		out := ansi.Strip(m.renderResourceSearch())
		lines := strings.Split(out, "\n")
		for i, l := range lines {
			if i > 0 && len([]rune(l)) != len([]rune(lines[0])) {
				t.Errorf("width=%d: line %d has length %d, want %d (box must stay rectangular)\nfull box:\n%s", width, i, len([]rune(l)), len([]rune(lines[0])), out)
			}
		}
	}

	m.width = 120
	out := ansi.Strip(m.renderResourceSearch())
	if !strings.Contains(out, "Resources") {
		t.Error("renderResourceSearch missing title")
	}
	wantCount := fmt.Sprintf("%d/%d", len(m.resources), len(m.resources))
	if !strings.Contains(out, wantCount) {
		t.Errorf("renderResourceSearch missing match count %q\nfull box:\n%s", wantCount, out)
	}
	for _, category := range []string{"Compute", "Network", "Gateways", "Storage", "Containers", "Database", "Governance"} {
		if !strings.Contains(out, category) {
			t.Errorf("renderResourceSearch missing category header %q\nfull box:\n%s", category, out)
		}
	}
}

// TestResourceSearchListStaysFixedSizeWhenFiltered locks in the requested
// change: narrowing the query should not shrink the box — it should stay
// the same fixed size (padded with blank rows), the way LazyVim's own file
// picker does, instead of shrinking to fit however few items match.
func TestResourceSearchListStaysFixedSizeWhenFiltered(t *testing.T) {
	m := Model{resources: registry.All(nil), width: 120, height: 40}
	m.openResourceSearch()

	unfiltered := strings.Split(m.renderResourceSearchList(m.resourceSearchListWidth()), "\n")

	m.picker.input.SetValue("instances")
	m.picker.refilter()
	if len(m.picker.filtered) == 0 || len(m.picker.filtered) >= len(unfiltered) {
		t.Fatalf("test setup: expected a query that narrows the list to a handful of items, got %d matches", len(m.picker.filtered))
	}
	filtered := strings.Split(m.renderResourceSearchList(m.resourceSearchListWidth()), "\n")

	if len(filtered) != len(unfiltered) {
		t.Errorf("filtered box has %d lines, unfiltered has %d — box height should stay fixed regardless of match count", len(filtered), len(unfiltered))
	}
}

// TestResourceSearchSplashYIsStableAcrossItemCount locks in the requested
// fix: the home screen's search box top position must not depend on how
// many resource kinds are registered — it used to slide upward every time
// one got added, since a taller box (more items) left less spare room
// above it (see resourceSearchY's splash case).
func TestResourceSearchSplashYIsStableAcrossItemCount(t *testing.T) {
	small := Model{resources: registry.All(nil)[:2], width: 120, height: 40}
	small.openResourceSearch()
	small.pickerReturnMode = modeSplash

	large := Model{resources: registry.All(nil), width: 120, height: 40}
	large.openResourceSearch()
	large.pickerReturnMode = modeSplash

	if got, want := small.resourceSearchY(), large.resourceSearchY(); got != want {
		t.Errorf("resourceSearchY() = %d with 2 resources, %d with %d — should be identical", got, want, len(large.resources))
	}

	// Pinned to the actual formula/constant, not just "the two happen to
	// match" — an earlier version of this fix anchored to
	// resourceSearchVisibleRows' cap (34) instead of
	// resourceSearchSplashAnchorItems (24), which also made the two cases
	// agree with each other but silently moved the box from its previously
	// approved position; equality alone wouldn't have caught that.
	wantY := (40 - (resourceSearchSplashAnchorItems + pickerResourceItemsTop + 2)) / 4
	if got := large.resourceSearchY(); got != wantY {
		t.Errorf("resourceSearchY() = %d, want %d (height=40, anchor=%d)", got, wantY, resourceSearchSplashAnchorItems)
	}
}

// TestResourceSearchListScrollsPastVisibleCap: once the picker has more
// rows (categories + resources) than resourceSearchVisibleRows fits, the
// list should scroll to keep the cursor on screen instead of continuing to
// grow the box — the row at the cursor must always be visible, and a row
// far above a cursor parked at the end must have scrolled out of view.
func TestResourceSearchListScrollsPastVisibleCap(t *testing.T) {
	m := Model{resources: registry.All(nil), width: 120, height: 24} // short terminal forces a small cap
	m.openResourceSearch()

	visible := resourceSearchVisibleRows(m)
	if visible >= len(m.picker.filtered) {
		t.Fatalf("test setup: visible rows (%d) should be smaller than the full list (%d) to actually exercise scrolling", visible, len(m.picker.filtered))
	}

	// Park the cursor on the last selectable item.
	for m.picker.cursor < len(m.picker.filtered)-1 {
		m.picker.cursorDown()
	}
	last := m.picker.filtered[m.picker.cursor]

	out := ansi.Strip(m.renderResourceSearchList(m.resourceSearchListWidth()))
	if !strings.Contains(out, last.label) {
		t.Errorf("cursor's own row (%q) should be visible after scrolling, got:\n%s", last.label, out)
	}
	if strings.Contains(out, "Compute") {
		t.Error(`the first category ("Compute") should have scrolled out of view with the cursor at the end of a capped list`)
	}
}

// TestOpenResourceSearchCursorStartsOnFirstResource: the cursor always
// starts on the first selectable resource (top of the list), not whichever
// resource is currently loaded — resIdx 0 defaults to Compartments, which
// resourceCategories now files last (Governance), so tracking "current"
// used to land the cursor at the bottom of the list on first open.
func TestOpenResourceSearchCursorStartsOnFirstResource(t *testing.T) {
	m := Model{resources: registry.All(nil)}
	m.openResourceSearch()

	if got, want := m.picker.filtered[m.picker.cursor].key, "instance"; got != want {
		t.Errorf("cursor lands on resource %q, want %q (the first entry under the first category)", got, want)
	}
}
