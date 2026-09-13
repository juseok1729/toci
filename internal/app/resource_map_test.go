package app

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func previewData() resourceMapData {
	return resourceMapData{
		vcnName: "hub-and-spoke",
		vcnCidr: "10.0.0.0/16",
		subnets: []mapNode{
			{id: "s1", label: "subnet-a"},
			{id: "s2", label: "subnet-b"},
		},
		routeTables: []mapNode{
			{id: "rt1", label: "rtb-1"},
			{id: "rt2", label: "rtb-2"},
		},
		connections: []mapNode{
			{id: "igw1", label: "igw-1"},
			{id: "nat1", label: "nat-1"},
		},
		subnetToRT: map[string]string{"s1": "rt1", "s2": "rt2"},
		rtToConn:   map[string][]string{"rt1": {"igw1"}, "rt2": {"nat1"}},
	}
}

func TestResourceMapPathNoSelection(t *testing.T) {
	data := previewData()
	subnetRow := map[string]int{"s1": 1, "s2": 5}
	rtRow := map[string]int{"rt1": 1, "rt2": 5}
	connRow := map[string]int{"igw1": 1, "nat1": 5}

	for _, selected := range []int{-1, 2, 99} {
		p := resourceMapPath(data, selected, subnetRow, rtRow, connRow)
		if len(p.boxes) != 0 || len(p.edgesA) != 0 || len(p.edgesB) != 0 {
			t.Errorf("resourceMapPath(selected=%d) = %+v, want zero-value (nothing highlighted)", selected, p)
		}
	}
}

func TestResourceMapPathResolvesSubnetToItsGateway(t *testing.T) {
	data := previewData()
	subnetRow := map[string]int{"s1": 1, "s2": 5}
	rtRow := map[string]int{"rt1": 1, "rt2": 5}
	connRow := map[string]int{"igw1": 1, "nat1": 5}

	// s1 -> rt1 -> igw1 only; s2/rt2/nat1 must not be pulled in.
	p := resourceMapPath(data, 0, subnetRow, rtRow, connRow)
	for _, want := range []string{"s1", "rt1", "igw1"} {
		if !p.boxes[want] {
			t.Errorf("boxes missing %q: %+v", want, p.boxes)
		}
	}
	for _, unwanted := range []string{"s2", "rt2", "nat1"} {
		if p.boxes[unwanted] {
			t.Errorf("boxes should not contain %q (belongs to a different subnet's path): %+v", unwanted, p.boxes)
		}
	}
	if want := []connectorEdge{{from: 1, to: 1}}; len(p.edgesA) != 1 || p.edgesA[0] != want[0] {
		t.Errorf("edgesA = %v, want %v", p.edgesA, want)
	}
	if want := []connectorEdge{{from: 1, to: 1}}; len(p.edgesB) != 1 || p.edgesB[0] != want[0] {
		t.Errorf("edgesB = %v, want %v", p.edgesB, want)
	}

	// Selecting the other subnet highlights the other path instead.
	p2 := resourceMapPath(data, 1, subnetRow, rtRow, connRow)
	if !p2.boxes["s2"] || !p2.boxes["rt2"] || !p2.boxes["nat1"] {
		t.Errorf("selecting s2 should highlight s2/rt2/nat1: %+v", p2.boxes)
	}
	if p2.boxes["s1"] || p2.boxes["rt1"] || p2.boxes["igw1"] {
		t.Errorf("selecting s2 should not highlight s1's path: %+v", p2.boxes)
	}
}

// highlightMarker is the "38;2;R;G;B" SGR fragment lipgloss embeds for
// ociHighlt — the hex color itself never appears verbatim in rendered
// output, and a full escape-sequence probe (render a marker rune, cut
// around it, the same trick state_color.go's selectedLinePrefix uses)
// isn't reliable here: mapBoxStyle bundles Bold in with the color on a
// highlighted box, producing "1;38;2;R;G;B" instead of a bare "38;2;R;G;B"
// — a substring match on just the RGB portion is what actually holds
// across both.
var highlightMarker = func() string {
	hex := strings.TrimPrefix(ociHighlt, "#")
	r, _ := strconv.ParseInt(hex[0:2], 16, 32)
	g, _ := strconv.ParseInt(hex[2:4], 16, 32)
	b, _ := strconv.ParseInt(hex[4:6], 16, 32)
	return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
}()

// TestRenderResourceMapHighlightsOnlySelectedSubnet is an end-to-end check
// that renderResourceMap actually applies the highlight: the selected
// subnet's own box renders in the accent color and the other one doesn't.
func TestRenderResourceMapHighlightsOnlySelectedSubnet(t *testing.T) {
	data := previewData()

	outNone := renderResourceMap(data, -1)
	if strings.Contains(outNone, highlightMarker) {
		t.Error("renderResourceMap(selected=-1) should have no highlighted color at all")
	}

	out0 := renderResourceMap(data, 0)
	if !strings.Contains(out0, highlightMarker) {
		t.Error("renderResourceMap(selected=0) should highlight something")
	}
	// subnet-b's own line must stay in the plain border color, not the
	// accent, when subnet-a (index 0) is the one selected.
	lines := strings.Split(out0, "\n")
	for _, l := range lines {
		if strings.Contains(l, "subnet-b") && strings.Contains(l, highlightMarker) {
			t.Errorf("subnet-b's line is highlighted when subnet-a (index 0) is selected: %q", l)
		}
	}
}
