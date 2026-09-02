package app

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/identity"

	"toci/internal/registry"
)

func TestFitColumnsGrowsProportionallyToFillSlack(t *testing.T) {
	cols := []registry.Column{
		{Header: "NAME", Width: 30},
		{Header: "DOMAIN(AD/FD)", Width: 14},
		{Header: "STATE", Width: 10},
	}
	colValues := [][]string{
		{"WYD-PRD-NGINX-CPT-01"},
		{"AD-1/FD-1"},
		{"RUNNING"},
	}

	widths := fitColumns(cols, colValues, 100)

	total := 0
	for i, w := range widths {
		natural := fitColumnWidth(cols[i].Header, colValues[i], cols[i].Width)
		if w < natural {
			t.Errorf("column %q width = %d, want >= its content-fit width %d", cols[i].Header, w, natural)
		}
		total += w + 2
	}
	if total != 100 {
		t.Errorf("total rendered width = %d, want exactly available (100) so the selected-row highlight reaches the box edge", total)
	}

	// STATE must not have absorbed a lopsided share of the slack (that's
	// what stretched colorizeState's text-cell padding across the
	// row) — every column should grow by roughly the same ratio.
	stateNatural := fitColumnWidth("STATE", colValues[2], 10)
	nameNatural := fitColumnWidth("NAME", colValues[0], 30)
	stateRatio := float64(widths[2]) / float64(stateNatural)
	nameRatio := float64(widths[0]) / float64(nameNatural)
	if d := stateRatio - nameRatio; d > 0.5 || d < -0.5 {
		t.Errorf("STATE grew by ratio %.2f vs NAME's %.2f, want roughly the same (proportional scaling)", stateRatio, nameRatio)
	}
}

func TestFitColumnsShrinksProportionallyWhenTooWide(t *testing.T) {
	cols := []registry.Column{
		{Header: "NAME", Width: 30},
		{Header: "DESCRIPTION", Width: 60},
	}
	colValues := [][]string{
		{"a-long-enough-name"},
		{"a very long description that would normally need sixty columns of room"},
	}

	widths := fitColumns(cols, colValues, 20)

	total := 0
	for _, w := range widths {
		if w < tableColMinWidth {
			t.Errorf("column width %d below the floor %d", w, tableColMinWidth)
		}
		total += w + 2
	}
	if total > 20 {
		t.Errorf("total rendered width = %d, want <= available (20)", total)
	}
}

func TestRelayoutPreservesCursorAndResizesColumns(t *testing.T) {
	m := New(nil, registry.Scope{Region: "ap-chuncheon-1"}, false, "demo", "dev")
	m.mode = modeTable
	m.width, m.height = 90, 24
	m.relayout()

	mk := func(n string) registry.Row {
		return registry.Row{ID: n, Name: n, Raw: identity.Compartment{Name: &n}}
	}
	m.rows = []registry.Row{mk("a"), mk("b"), mk("c")}
	m.setDisplayRows()
	m.table.SetCursor(2)

	widthNarrow := m.mainContentWidth()
	m.width = 150
	m.relayout()
	widthWide := m.mainContentWidth()

	if widthWide <= widthNarrow {
		t.Fatalf("mainContentWidth wide=%d, narrow=%d — a wider terminal should free up width", widthWide, widthNarrow)
	}
	if m.table.Width() != widthWide-tableBoxOverhead {
		t.Errorf("table.Width() = %d after resizing, want %d (mainContentWidth minus the box border)", m.table.Width(), widthWide-tableBoxOverhead)
	}
	if got := m.table.Cursor(); got != 2 {
		t.Errorf("table cursor = %d after relayout, want 2 (relayout must not reset the selection)", got)
	}
}

func TestFitColumnsExactFitReturnsNatural(t *testing.T) {
	cols := []registry.Column{{Header: "NAME", Width: 30}}
	colValues := [][]string{{"x"}}

	natural := fitColumnWidth("NAME", colValues[0], 30)
	widths := fitColumns(cols, colValues, natural+2)

	if widths[0] != natural {
		t.Errorf("widths[0] = %d, want unchanged natural width %d when already an exact fit", widths[0], natural)
	}
}

// TestFitColumnWidthCeilingTruncatesPastDeclaredWidth pins down the
// mechanism behind a real bug: DB System's ROLE column was declared
// Width: 10 (fine for "Primary"/"Standby"/"RAC"/"-"), but a cross-region
// Data Guard peer appends "→<region>" (fetchDbSystemRole), producing values
// up to 25 chars ("Standby→af-johannesburg-1"). fitColumnWidth's ceiling
// parameter is a hard cap, not a hint — it truncates rather than growing to
// fit unexpectedly long content, so the declared Width has to cover the
// true worst case up front.
func TestFitColumnWidthCeilingTruncatesPastDeclaredWidth(t *testing.T) {
	longRole := "Standby→af-johannesburg-1" // 25 chars — the real worst case
	values := []string{"Primary", "RAC", "-", longRole}

	if got := fitColumnWidth("ROLE", values, 10); got != 10 {
		t.Errorf("fitColumnWidth with too-small ceiling 10 = %d, want capped at 10 (demonstrates the truncation bug)", got)
	}
	wantWidth := len([]rune(longRole)) // fitColumnWidth counts runes, not bytes — "→" is multi-byte
	if got := fitColumnWidth("ROLE", values, 25); got != wantWidth {
		t.Errorf("fitColumnWidth with ceiling 25 = %d, want %d (fits the real longest value, matching db_system.go's fixed Width)", got, wantWidth)
	}
}
