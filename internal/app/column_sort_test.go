package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/oracle/oci-go-sdk/v65/identity"

	"toci/internal/registry"
)

// compartmentModelForSortTest builds a Model sitting on the Compartments
// resource (NAME, DESCRIPTION — two plain-text columns) with three rows out
// of alphabetical order, the setup every test below needs to exercise
// left/right/"o" (updateTable) and applyColumnSort (setDisplayRows).
func compartmentModelForSortTest(t *testing.T) Model {
	t.Helper()
	m := New(nil, registry.Scope{Region: "ap-chuncheon-1"}, false, "demo", "dev")
	m.mode = modeTable
	m.width, m.height = 100, 30
	m.relayout()
	for i, r := range m.resources {
		if r.Key() == "compartment" {
			m.resIdx = i
			break
		}
	}
	mk := func(n string) registry.Row {
		return registry.Row{ID: n, Name: n, Raw: identity.Compartment{Name: &n}}
	}
	m.rows = []registry.Row{mk("charlie"), mk("alpha"), mk("bravo")}
	m.setDisplayRows()
	return m
}

func namesOf(rows []registry.Row) []string {
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name
	}
	return names
}

func TestCompareCellTextNumericColumnsSortNumericallyNotLexically(t *testing.T) {
	// Plain lexical comparison would put "10" before "2" — numeric columns
	// (OCPU, MEM(GB), ...) need to sort by value instead.
	if c := compareCellText("2", "10"); c >= 0 {
		t.Errorf("compareCellText(2, 10) = %d, want < 0 (2 sorts before 10 numerically)", c)
	}
	if c := compareCellText("10", "2"); c <= 0 {
		t.Errorf("compareCellText(10, 2) = %d, want > 0", c)
	}
	if c := compareCellText("4.5", "4.5"); c != 0 {
		t.Errorf("compareCellText(4.5, 4.5) = %d, want 0", c)
	}
}

func TestCompareCellTextFallsBackToCaseInsensitiveStringCompare(t *testing.T) {
	if c := compareCellText("Alpha", "bravo"); c >= 0 {
		t.Errorf("compareCellText(Alpha, bravo) = %d, want < 0", c)
	}
	if c := compareCellText("alpha", "ALPHA"); c != 0 {
		t.Errorf("compareCellText(alpha, ALPHA) = %d, want 0 (case-insensitive)", c)
	}
}

func TestApplyColumnSortInactiveLeavesNaturalOrder(t *testing.T) {
	m := compartmentModelForSortTest(t)
	got := namesOf(m.displayRows)
	want := []string{"charlie", "alpha", "bravo"}
	if len(got) != len(want) {
		t.Fatalf("displayRows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("displayRows = %v, want %v (natural List() order, unsorted)", got, want)
		}
	}
}

func TestApplyColumnSortOrdersAscendingThenDescending(t *testing.T) {
	m := compartmentModelForSortTest(t)
	m.sortActive = true
	m.sortCol = 0 // NAME
	m.sortDesc = false
	m.setDisplayRows()
	if got := namesOf(m.displayRows); got[0] != "alpha" || got[1] != "bravo" || got[2] != "charlie" {
		t.Errorf("ascending sort = %v, want [alpha bravo charlie]", got)
	}

	m.sortDesc = true
	m.setDisplayRows()
	if got := namesOf(m.displayRows); got[0] != "charlie" || got[1] != "bravo" || got[2] != "alpha" {
		t.Errorf("descending sort = %v, want [charlie bravo alpha]", got)
	}
}

func TestUpdateTableLeftRightMovesSortCursorColumnClamped(t *testing.T) {
	m := compartmentModelForSortTest(t)
	if n := len(m.displayColumns()); n != 2 {
		t.Fatalf("test setup: compartment has %d columns, want 2 (NAME, DESCRIPTION)", n)
	}
	if m.sortCursorCol != 0 {
		t.Fatalf("test setup: sortCursorCol = %d, want 0", m.sortCursorCol)
	}

	mi, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m2 := mi.(Model)
	if m2.sortCursorCol != 0 {
		t.Errorf("left at column 0 = %d, want clamped to 0", m2.sortCursorCol)
	}

	mi, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m3 := mi.(Model)
	if m3.sortCursorCol != 1 {
		t.Errorf("right from column 0 = %d, want 1", m3.sortCursorCol)
	}

	mi, _ = m3.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m4 := mi.(Model)
	if m4.sortCursorCol != 1 {
		t.Errorf("right at last column = %d, want clamped to 1", m4.sortCursorCol)
	}

	mi, _ = m4.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m5 := mi.(Model)
	if m5.sortCursorCol != 0 {
		t.Errorf("left from column 1 = %d, want 0", m5.sortCursorCol)
	}
}

func TestUpdateTableOCyclesSortAscendingDescendingOff(t *testing.T) {
	m := compartmentModelForSortTest(t)
	oMsg := tea.KeyPressMsg{Code: 'o', Text: "o"}

	mi, _ := m.Update(oMsg)
	m2 := mi.(Model)
	if !m2.sortActive || m2.sortCol != 0 || m2.sortDesc {
		t.Fatalf("after first \"o\": sortActive=%v sortCol=%d sortDesc=%v, want true,0,false", m2.sortActive, m2.sortCol, m2.sortDesc)
	}
	if got := namesOf(m2.displayRows); got[0] != "alpha" {
		t.Errorf("displayRows after ascending sort = %v, want alpha first", got)
	}

	mi, _ = m2.Update(oMsg)
	m3 := mi.(Model)
	if !m3.sortActive || m3.sortCol != 0 || !m3.sortDesc {
		t.Fatalf("after second \"o\": sortActive=%v sortCol=%d sortDesc=%v, want true,0,true", m3.sortActive, m3.sortCol, m3.sortDesc)
	}
	if got := namesOf(m3.displayRows); got[0] != "charlie" {
		t.Errorf("displayRows after descending sort = %v, want charlie first", got)
	}

	mi, _ = m3.Update(oMsg)
	m4 := mi.(Model)
	if m4.sortActive {
		t.Fatalf("after third \"o\": sortActive=%v, want false (cycled back to unsorted)", m4.sortActive)
	}
	if got := namesOf(m4.displayRows); got[0] != "charlie" || got[1] != "alpha" || got[2] != "bravo" {
		t.Errorf("displayRows after sort cleared = %v, want natural order restored", got)
	}
}

func TestUpdateTableOOnDifferentColumnStartsFreshAscending(t *testing.T) {
	m := compartmentModelForSortTest(t)
	m.sortActive = true
	m.sortCol = 0
	m.sortDesc = true // descending on column 0
	m.sortCursorCol = 1

	mi, _ := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m2 := mi.(Model)
	if !m2.sortActive || m2.sortCol != 1 || m2.sortDesc {
		t.Errorf("\"o\" on a new column: sortActive=%v sortCol=%d sortDesc=%v, want true,1,false (fresh ascending, not carrying over the old column's descending state)", m2.sortActive, m2.sortCol, m2.sortDesc)
	}
}

// TestReloadCurrentResetsSortState reproduces the reasoning behind clearing
// sort on every resource switch: a column index carries no meaning across
// resource kinds (column 1 is DESCRIPTION here, but something unrelated on
// another resource), so switching resources must not silently keep sorting
// by whatever index happened to be active.
func TestReloadCurrentResetsSortState(t *testing.T) {
	m := compartmentModelForSortTest(t)
	m.sortActive = true
	m.sortCol = 1
	m.sortCursorCol = 1

	m.reloadCurrent()

	if m.sortActive {
		t.Errorf("sortActive = true after reloadCurrent, want false")
	}
	if m.sortCursorCol != 0 {
		t.Errorf("sortCursorCol = %d after reloadCurrent, want reset to 0", m.sortCursorCol)
	}
}

// TestApplyColumnSortAppliesInSubtreeModeToo reproduces a reported gap:
// column sort was skipped entirely while subtree mode ("C") was on. The
// already-loaded rows should still sort like anywhere else; only the "—
// loading —" placeholders for compartments that haven't reported back yet
// stay pinned at the end, since sorting placeholder text wouldn't mean
// anything and would fight NAME sort logic; SUBTREE mode also prepends a
// COMPARTMENT column, so sorting by NAME here is column index 1.
func TestApplyColumnSortAppliesInSubtreeModeToo(t *testing.T) {
	m := New(nil, registry.Scope{Region: "ap-chuncheon-1"}, false, "demo", "dev")
	m.mode = modeTable
	m.width, m.height = 100, 30
	m.relayout()
	for i, r := range m.resources {
		if r.Key() == "compartment" {
			m.resIdx = i
			break
		}
	}
	mk := func(n string) registry.Row {
		return registry.Row{ID: n, Name: n, Raw: identity.Compartment{Name: &n}}
	}
	m.subtreeOn = true
	m.subtreeTargets = []subtreeTarget{{ID: "done"}, {ID: "pending"}}
	m.subtreeDone = map[string]bool{"done": true}
	m.subtreeRows = map[string][]registry.Row{"done": {mk("charlie"), mk("alpha"), mk("bravo")}}

	if got := len(m.displayColumns()); got != 3 {
		t.Fatalf("test setup: subtree mode has %d columns, want 3 (COMPARTMENT, NAME, DESCRIPTION)", got)
	}
	m.sortActive = true
	m.sortCol = 1 // NAME, after subtree mode's prepended COMPARTMENT column
	m.sortDesc = false
	m.setDisplayRows()

	got := namesOf(m.displayRows)
	want := []string{"alpha", "bravo", "charlie", "— loading —"}
	if len(got) != len(want) {
		t.Fatalf("displayRows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("displayRows = %v, want %v (real rows sorted, placeholder pinned last)", got, want)
			break
		}
	}
}
