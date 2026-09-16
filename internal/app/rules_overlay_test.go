package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"toci/internal/registry"
)

// TestOpenRulesOverlayFloatsOverTheTable checks the requested UX change: a
// "v" rules view (security-list/route-table/nsg/drg-route-table) should
// float over the table at the bottom of the screen, the same treatment the
// "M" resource map already gets, instead of replacing the whole screen the
// way the plain "d" YAML detail view does.
func TestOpenRulesOverlayFloatsOverTheTable(t *testing.T) {
	m := Model{resources: registry.All(nil), width: 160, height: 45, mode: modeTable, table: newTable(20)}
	m.tableHeight = m.height - 10
	m.table.SetHeight(m.tableHeight)
	m.relayout()

	m.openRulesOverlay("Rule 1: TCP 0.0.0.0/0 -> 22", &detailExportData{filenameSuffix: "test", header: []string{"A"}, records: [][]string{{"x"}}})

	if m.mode != modeDetail || !m.rulesOverlay {
		t.Fatalf("mode = %v, rulesOverlay = %v, want modeDetail/true", m.mode, m.rulesOverlay)
	}

	out := ansi.Strip(m.viewContent())
	if !strings.Contains(out, "Profile:") {
		t.Error("viewContent should still show the table header behind the rules overlay, but it's gone")
	}
	if !strings.Contains(out, "Rule 1: TCP 0.0.0.0/0 -> 22") {
		t.Error("viewContent is missing the rules content")
	}
	if !strings.Contains(out, "esc/v close") {
		t.Error("viewContent is missing the overlay box's own title/hint")
	}

	// Esc closes it and restores full-page detail sizing for whatever
	// opens in modeDetail next (see updateDetail's "wasOverlay" handling).
	mi, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m2 := mi.(Model)
	if m2.mode != modeTable || m2.rulesOverlay {
		t.Errorf("mode = %v, rulesOverlay = %v after Esc, want modeTable/false", m2.mode, m2.rulesOverlay)
	}
	if m2.detail.Width() != m2.mainContentWidth() {
		t.Errorf("detail width after closing the overlay = %d, want full-page width %d", m2.detail.Width(), m2.mainContentWidth())
	}
}
