package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMapColWidth(t *testing.T) {
	items := []mapNode{{label: "short"}, {label: "a-much-longer-resource-name"}}
	if got, want := mapColWidth(items, 30), len("a-much-longer-resource-name"); got != want {
		t.Errorf("mapColWidth() = %d, want %d (longest label)", got, want)
	}
	if got := mapColWidth(items, 10); got != 10 {
		t.Errorf("mapColWidth() with ceiling 10 = %d, want capped at 10", got)
	}
	if got := mapColWidth(nil, 30); got != 8 {
		t.Errorf("mapColWidth(nil) = %d, want the floor 8", got)
	}
}

// TestMapColWidthMeasuresLinesSeparately guards a subtle bug: a label can
// carry a second line (a subnet's CIDR, a NAT gateway's IP) joined with
// "\n" — measuring the whole label's rune count instead of each line
// separately would count that newline and the second line as if they
// were more characters tacked onto the first line, sizing the column far
// wider than either line actually needs.
func TestMapColWidthMeasuresLinesSeparately(t *testing.T) {
	items := []mapNode{{label: "subnet-a\n10.0.1.0/24"}} // longest line is "10.0.1.0/24" (11 chars)
	if got, want := mapColWidth(items, 30), 11; got != want {
		t.Errorf("mapColWidth() = %d, want %d (the longest single line, not the whole label's length)", got, want)
	}
}

func TestRenderMapColumnRowBookkeeping(t *testing.T) {
	items := []mapNode{
		{id: "s1", label: "subnet-1"},
		{id: "s2", label: "subnet-2"},
	}
	lines, rowOf := renderMapColumn(items, 20, nil)

	// box 1: rows 0-2 (border/content/border), blank spacer row 3, box 2: rows 4-6.
	if rowOf["s1"] != 1 {
		t.Errorf("rowOf[s1] = %d, want 1 (first box's content line)", rowOf["s1"])
	}
	if rowOf["s2"] != 5 {
		t.Errorf("rowOf[s2] = %d, want 5 (second box's content line, after the spacer)", rowOf["s2"])
	}
	if len(lines) != 7 {
		t.Errorf("len(lines) = %d, want 7 (3+1+3)", len(lines))
	}
	if !strings.Contains(ansi.Strip(lines[rowOf["s1"]]), "subnet-1") {
		t.Errorf("lines[rowOf[s1]] = %q, want it to contain the label", lines[rowOf["s1"]])
	}
}

// detailYellowMarker is the SGR fragment lipgloss emits for stateTextWarn's
// Bold + Color("3") — a basic ANSI color index, not truecolor or a 256
// palette index, so the escape shape is the plain "33" foreground code
// (bundled with bold as "1;33"), unlike a hex color's "38;2;R;G;B".
const detailYellowMarker = "1;33"

// TestRenderMapColumnDetailLineIsYellowWhenNotHighlighted is the reported
// fix: a subnet's CIDR (or a NAT gateway's IP) used to inherit the
// terminal's plain default text color, easy to miss against the box's
// already-muted border — the detail line renders in stateTextWarn's
// yellow unless the box is on the highlighted path (where the whole box
// goes uniformly gold instead, detail line included — see
// renderMapColumn's own comment on why that one can't also be pre-styled).
func TestRenderMapColumnDetailLineIsYellowWhenNotHighlighted(t *testing.T) {
	items := []mapNode{{id: "s1", label: "subnet-a", detail: "10.0.1.0/24"}}

	lines, rowOf := renderMapColumn(items, 20, nil)
	if len(lines) != 4 { // border, label, detail, border
		t.Fatalf("len(lines) = %d, want 4 (a detail line adds one row)", len(lines))
	}
	if rowOf["s1"] != 1 {
		t.Errorf("rowOf[s1] = %d, want 1 (still the label line, not the detail line below it)", rowOf["s1"])
	}
	detailLine := lines[rowOf["s1"]+1]
	if !strings.Contains(ansi.Strip(detailLine), "10.0.1.0/24") {
		t.Fatalf("detail line = %q, want it to contain the CIDR", detailLine)
	}
	if !strings.Contains(detailLine, detailYellowMarker) {
		t.Errorf("detail line = %q, want it styled yellow (%s)", detailLine, detailYellowMarker)
	}

	highlighted, rowOf2 := renderMapColumn(items, 20, map[string]bool{"s1": true})
	detailLineHL := highlighted[rowOf2["s1"]+1]
	if strings.Contains(detailLineHL, detailYellowMarker) {
		t.Errorf("highlighted detail line = %q, should not carry its own yellow styling (the whole box is gold instead)", detailLineHL)
	}
}

func TestRenderMapColumnGroupHeaders(t *testing.T) {
	items := []mapNode{
		{id: "s1", label: "subnet-1", group: "AD-1"},
		{id: "s2", label: "subnet-2", group: "AD-1"},
		{id: "s3", label: "subnet-3", group: "AD-2"},
	}
	lines, _ := renderMapColumn(items, 20, nil)

	joined := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Count(joined, "AD-1") != 1 {
		t.Errorf("group header AD-1 should appear exactly once (only before its first item), got:\n%s", joined)
	}
	if strings.Count(joined, "AD-2") != 1 {
		t.Errorf("group header AD-2 should appear exactly once, got:\n%s", joined)
	}
}

func TestBuildEdgesSkipsUnresolvedTargets(t *testing.T) {
	fromRow := map[string]int{"s1": 1, "s2": 5}
	targets := map[string][]string{
		"s1": {"rt1"},
		"s2": {"rt-missing"}, // no row for this — must be dropped, not guessed at
	}
	toRow := map[string]int{"rt1": 2}

	edges := buildEdges(fromRow, targets, toRow)
	want := []connectorEdge{{from: 1, to: 2}}
	if !reflect.DeepEqual(edges, want) {
		t.Errorf("buildEdges() = %v, want %v", edges, want)
	}
}

func TestRenderConnectorBusNoEdges(t *testing.T) {
	lines := renderConnectorBus(nil, 3)
	for i, l := range lines {
		if l != "   " {
			t.Errorf("line %d = %q, want a blank 3-char gap", i, l)
		}
	}
}

func TestRenderConnectorBusStraightLine(t *testing.T) {
	lines := renderConnectorBus([]connectorEdge{{from: 2, to: 2}}, 5)
	want := []string{"   ", "   ", "───", "   ", "   "}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("renderConnectorBus() = %v, want %v (a lone 1:1 edge on the same row is a plain line)", lines, want)
	}
}

// TestRenderConnectorBusMergesManyToOne is the shape a subnet column feeding
// into a shared route table needs: several "from" rows, one "to" row, all
// riding a single spine between the min and max involved row.
func TestRenderConnectorBusMergesManyToOne(t *testing.T) {
	edges := []connectorEdge{{from: 0, to: 4}, {from: 2, to: 4}, {from: 4, to: 4}}
	lines := renderConnectorBus(edges, 5)

	if lines[0] != "─┤ " {
		t.Errorf("row 0 (topmost from-row) = %q, want %q", lines[0], "─┤ ")
	}
	if lines[1] != " │ " {
		t.Errorf("row 1 (pass-through) = %q, want %q", lines[1], " │ ")
	}
	if lines[2] != "─┤ " {
		t.Errorf("row 2 (a from-row) = %q, want %q", lines[2], "─┤ ")
	}
	if lines[4] != "─┼─" {
		t.Errorf("row 4 (both a from-row and the to-row) = %q, want %q", lines[4], "─┼─")
	}
}

// TestRenderConnectorBusSplitsOneToMany is the mirror case: a route table
// fanning out to several gateways.
func TestRenderConnectorBusSplitsOneToMany(t *testing.T) {
	edges := []connectorEdge{{from: 1, to: 0}, {from: 1, to: 3}}
	lines := renderConnectorBus(edges, 4)

	if lines[0] != " ├─" {
		t.Errorf("row 0 (a to-row) = %q, want %q", lines[0], " ├─")
	}
	// Row 1 is only ever a from-row here (the route table's own row) — the
	// stub direction reflects which side of the bus has a box at that
	// row, not data flow, so it's a left stub (reaching back to the route
	// table) even though this row is the one "sending" two connections out.
	if lines[1] != "─┤ " {
		t.Errorf("row 1 (the shared from-row) = %q, want %q", lines[1], "─┤ ")
	}
	if lines[3] != " ├─" {
		t.Errorf("row 3 (the other to-row) = %q, want %q", lines[3], " ├─")
	}
}

func TestPadLines(t *testing.T) {
	got := padLines([]string{"a", "b"}, 4)
	want := []string{"a", "b", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("padLines() = %v, want %v", got, want)
	}
	// Already at (or past) the target height — left alone.
	got = padLines([]string{"a", "b", "c"}, 2)
	want = []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("padLines() on an already-tall slice = %v, want %v (unchanged)", got, want)
	}
}

// TestApplyBusHighlight is the "select a subnet" feature's coloring rule:
// rows the highlighted edge's [from, to] span touches render in the
// accent style, everything else in the plain border-matching one.
func TestApplyBusHighlight(t *testing.T) {
	lines := renderConnectorBus([]connectorEdge{{from: 0, to: 3}}, 4)
	out := applyBusHighlight(lines, []connectorEdge{{from: 0, to: 3}})

	for i, l := range out {
		plain := ansi.Strip(l)
		if plain != lines[i] {
			t.Errorf("row %d: applyBusHighlight changed the text (%q -> %q), want only styling to change", i, lines[i], plain)
		}
		if !strings.Contains(l, "\x1b[") {
			t.Errorf("row %d = %q, want ANSI styling applied", i, l)
		}
	}
}

func TestApplyBusHighlightNoHighlightEdges(t *testing.T) {
	lines := renderConnectorBus([]connectorEdge{{from: 0, to: 2}}, 3)
	out := applyBusHighlight(lines, nil)
	for i, l := range out {
		if ansi.Strip(l) != lines[i] {
			t.Errorf("row %d text changed: %q -> %q", i, lines[i], ansi.Strip(l))
		}
	}
	// With no highlight edges, every row should render in mapBusStyle, not
	// mapBusHighlightStyle — spot-check by re-deriving the expected string.
	if want := mapBusStyle.Render(lines[0]); out[0] != want {
		t.Errorf("row 0 = %q, want the plain bus style %q", out[0], want)
	}
}
