package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

func TestStateStyleForPicksCorrectTier(t *testing.T) {
	const noTier = "none"
	cases := map[string]string{
		"Running":                   "good",
		"Active":                    "good",
		"Available":                 "good",
		"Standby":                   "good",
		"Stopped":                   "bad",
		"Failed":                    "bad",
		"Inaccessible":              "bad",
		"Unavailable":               "bad",
		"Restore Failed":            "bad",
		"Needs Attention":           "warn",
		"Available Needs Attention": "warn", // contains "Available" too — Warn must win
		"Provisioning":              noTier, // transitional — no tier
		"Terminated":                noTier,
	}
	tiers := map[string]lipgloss.Style{"good": stateTextGood, "bad": stateTextBad, "warn": stateTextWarn}

	for value, want := range cases {
		got, ok := stateStyleFor(value, false)
		if want == noTier {
			if ok {
				t.Errorf("stateStyleFor(%q) matched a tier, want none (transitional)", value)
			}
			continue
		}
		if !ok {
			t.Errorf("stateStyleFor(%q) matched no tier, want %q", value, want)
			continue
		}
		if !reflect.DeepEqual(got, tiers[want]) {
			t.Errorf("stateStyleFor(%q) picked the wrong tier, want %q", value, want)
		}
	}
}

func TestColorizeStateColorsTextNotBackground(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	cols := []table.Column{{Title: "NAME", Width: 6}, {Title: "STATE", Width: 9}, {Title: "TAIL", Width: 4}}
	plain := "web11   Running   xyz "

	// Non-selected row: just the plain text, no wrapping style.
	out := colorizeState("HEADER\n"+plain, cols)
	lines := strings.Split(out, "\n")
	if ansi.Strip(lines[1]) != plain {
		t.Fatalf("content changed: got %q, want %q", ansi.Strip(lines[1]), plain)
	}
	start, end, _ := columnRange(cols, "STATE")
	mid := ansi.Cut(lines[1], start, end)
	if !strings.Contains(mid, "Running") || mid == ansi.Cut(plain, start, end) {
		t.Errorf("STATE cell wasn't restyled on a non-selected row: %q", mid)
	}
	// selBgParams below shouldn't appear anywhere on a non-selected row —
	// there's no row highlight to preserve, so no Background should get
	// introduced.
	selBgParams := "48;2;56;104;72" // ociSelBg (#386848) as a truecolor SGR background param
	if strings.Contains(lines[1], selBgParams) {
		t.Errorf("non-selected row unexpectedly carries the selected-row background: %q", lines[1])
	}

	// Selected row: bubbles wraps the whole rendered line in selStyle before
	// colorizeState ever sees it.
	selLine := selStyle.Render(plain)
	out = colorizeState("HEADER\n"+selLine, cols)
	lines = strings.Split(out, "\n")
	if ansi.Strip(lines[1]) != plain {
		t.Fatalf("selected row content changed: got %q, want %q", ansi.Strip(lines[1]), plain)
	}
	// The selected row's own background must still be active right after
	// the STATE cell — otherwise recoloring STATE's text punches a
	// default-background hole in the middle of the row highlight.
	afterState := ansi.Cut(lines[1], end, 1<<20)
	if !strings.Contains(afterState, selBgParams) {
		t.Errorf("selected-row background not carried past the STATE cell: %q", afterState)
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
