package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"toci/internal/registry"
)

func TestColumnRangeFindsNamedColumn(t *testing.T) {
	cols := []table.Column{{Title: "NAME", Width: 10}, {Title: "STATE", Width: 8}}
	start, end, ok := columnRange(cols, "STATE")
	if !ok || start != 12 || end != 22 {
		t.Fatalf("columnRange(STATE) = %d,%d,%v, want 12,22,true", start, end, ok)
	}
	if _, _, ok := columnRange(cols, "MISSING"); ok {
		t.Fatalf("columnRange(MISSING) = ok, want not found")
	}
}

func TestBlinkRecentRowsOnlyPaintsMatchingRowsWhenOn(t *testing.T) {
	// Force real ANSI output — lipgloss auto-disables color with no TTY
	// attached, which a `go test` run never has.
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	cols := []table.Column{{Title: "NAME", Width: 6}, {Title: "STATE", Width: 7}}
	view := "HEADER\n" +
		"web11   RUNNING\n" +
		"web12   RUNNING\n"
	recent := map[string]bool{"web11": true}

	if got := blinkRecentRows(view, cols, recent, false); got != view {
		t.Fatalf("blinkOn=false must not alter the view, got %q", got)
	}

	got := blinkRecentRows(view, cols, recent, true)
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[1], "web11") || lines[1] == "web11   RUNNING" {
		t.Errorf("recent row 1 (web11) not styled: %q", lines[1])
	}
	if lines[2] != "web12   RUNNING" {
		t.Errorf("non-recent row 2 (web12) should be untouched, got %q", lines[2])
	}
}

func TestIsRecentRowChecksCreationWindow(t *testing.T) {
	var m Model

	fresh := registry.Row{ID: "ocid.fresh", TimeCreated: time.Now().Add(-time.Hour)}
	stale := registry.Row{ID: "ocid.stale", TimeCreated: time.Now().Add(-30 * 24 * time.Hour)}

	if !m.isRecentRow(fresh) {
		t.Error("row created 1h ago should be recent")
	}
	if m.isRecentRow(stale) {
		t.Error("row created 30 days ago should not be recent")
	}
}
