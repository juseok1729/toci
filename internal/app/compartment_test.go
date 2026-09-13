package app

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/registry"
)

// buildTestTree builds a small fixture tree:
//
//	root
//	└─ a
//	   ├─ b
//	   │  └─ d
//	   └─ c
func buildTestTree() *compartmentTree {
	root := &compartmentNode{ID: "root", Name: "tenancy"}
	a := &compartmentNode{ID: "a", Name: "a", ParentID: "root"}
	b := &compartmentNode{ID: "b", Name: "b", ParentID: "a"}
	c := &compartmentNode{ID: "c", Name: "c", ParentID: "a", Description: "the c compartment"}
	d := &compartmentNode{ID: "d", Name: "d", ParentID: "b"}
	root.Children = []*compartmentNode{a}
	a.Children = []*compartmentNode{b, c}
	b.Children = []*compartmentNode{d}
	byID := map[string]*compartmentNode{"root": root, "a": a, "b": b, "c": c, "d": d}
	return &compartmentTree{root: root, byID: byID}
}

func TestCompartmentTreePathTo(t *testing.T) {
	tree := buildTestTree()
	got := tree.pathTo("d")
	want := []string{"tenancy", "a", "b", "d"}
	if len(got) != len(want) {
		t.Fatalf("pathTo(d) = %v, want names %v", got, want)
	}
	for i, c := range got {
		if c.Name != want[i] {
			t.Errorf("pathTo(d)[%d].Name = %q, want %q", i, c.Name, want[i])
		}
	}
}

func TestCompartmentTreeDescendantsAndSubtreeCount(t *testing.T) {
	tree := buildTestTree()
	a := tree.node("a")
	if got, want := a.subtreeCount(), 3; got != want { // b, c, d
		t.Errorf("a.subtreeCount() = %d, want %d", got, want)
	}
	b := tree.node("b")
	if got, want := b.subtreeCount(), 1; got != want { // d
		t.Errorf("b.subtreeCount() = %d, want %d", got, want)
	}
}

func TestCompartmentTreeRelativePath(t *testing.T) {
	tree := buildTestTree()
	if got, want := tree.relativePath("a", "d"), "b/d"; got != want {
		t.Errorf("relativePath(a, d) = %q, want %q", got, want)
	}
	if got, want := tree.relativePath("a", "c"), "c"; got != want {
		t.Errorf("relativePath(a, c) = %q, want %q", got, want)
	}
	if got := tree.relativePath("a", "a"); got != "" {
		t.Errorf("relativePath(a, a) = %q, want empty", got)
	}
	if got := tree.relativePath("c", "d"); got != "" {
		t.Errorf("relativePath(c, d) (d not under c) = %q, want empty", got)
	}
}

// TestCompartmentPickerItemsKeepsAncestorsVisible is the F5 requirement
// that a fuzzy match on a deep node keeps its ancestor chain in the
// filtered list, so same-named compartments stay distinguishable by path.
func TestCompartmentPickerItemsKeepsAncestorsVisible(t *testing.T) {
	tree := buildTestTree()
	items := compartmentPickerItems(tree, "d", nil, "a")

	byID := map[string]bool{}
	for _, it := range items {
		byID[it.key] = true
	}
	for _, id := range []string{"root", "a", "b", "d"} {
		if !byID[id] {
			t.Errorf("filtering for %q dropped ancestor %q from the result: %v", "d", id, items)
		}
	}
	if byID["c"] {
		t.Errorf("filtering for %q should have excluded unrelated sibling %q", "d", "c")
	}

	for _, it := range items {
		if it.key == "a" && !it.isCurrent {
			t.Errorf("node %q should be marked isCurrent (currentID matched)", it.key)
		}
	}
}

func TestCompartmentPickerItemsEmptyQueryShowsWholeTree(t *testing.T) {
	tree := buildTestTree()
	items := compartmentPickerItems(tree, "", nil, "root")
	if got, want := len(items), 5; got != want {
		t.Fatalf("len(items) = %d, want %d (whole tree)", got, want)
	}
}

func TestPushRecentMRUOrderAndDedup(t *testing.T) {
	var list []recentEntry
	list = pushRecent(list, "a", "A", false)
	list = pushRecent(list, "b", "B", false)
	list = pushRecent(list, "a", "A", true) // re-visiting "a" moves it to the front, updates Subtree

	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2 (revisit should dedup, not append)", len(list))
	}
	if list[0].ID != "a" || !list[0].Subtree {
		t.Errorf("list[0] = %+v, want {a ... Subtree:true} at the MRU head", list[0])
	}
	if list[1].ID != "b" {
		t.Errorf("list[1].ID = %q, want %q", list[1].ID, "b")
	}
}

func TestTogglePinSurvivesTrim(t *testing.T) {
	var list []recentEntry
	list = togglePin(list, "pinned", "Pinned")
	for i := 0; i < recentMaxSlots+3; i++ {
		list = pushRecent(list, string(rune('A'+i)), string(rune('A'+i)), false)
	}
	found := false
	for _, e := range list {
		if e.ID == "pinned" {
			found = true
		}
	}
	if !found {
		t.Errorf("pinned entry was evicted by MRU overflow: %v", list)
	}
	if len(list) > recentMaxSlots {
		t.Errorf("len(list) = %d, want <= %d", len(list), recentMaxSlots)
	}
}

func TestSwitchCompartmentResetsSubScopeAndUpdatesRecent(t *testing.T) {
	m := Model{
		resources:     registry.All(nil),
		scope:         registry.Scope{Region: "us-ashburn-1", CompartmentID: "a", VcnID: "vcn1"},
		vcnFilterName: "my-vcn",
		table:         newTable(20),
		compTree:      buildTestTree(),
	}

	cmd := m.switchCompartment("c", "c", nil)
	if cmd == nil {
		t.Fatal("switchCompartment returned a nil Cmd (expected a load Cmd)")
	}
	if m.scope.CompartmentID != "c" {
		t.Errorf("scope.CompartmentID = %q, want %q", m.scope.CompartmentID, "c")
	}
	if m.scope.VcnID != "" || m.vcnFilterName != "" {
		t.Errorf("VCN sub-scope not reset: VcnID=%q vcnFilterName=%q", m.scope.VcnID, m.vcnFilterName)
	}
	if len(m.recentList) != 1 || m.recentList[0].ID != "c" {
		t.Errorf("recentList = %v, want a single entry for %q", m.recentList, "c")
	}
	wantPath := []string{"tenancy", "a", "c"}
	if len(m.compPath) != len(wantPath) {
		t.Fatalf("compPath = %v, want names %v", m.compPath, wantPath)
	}
	for i, c := range m.compPath {
		if c.Name != wantPath[i] {
			t.Errorf("compPath[%d].Name = %q, want %q", i, c.Name, wantPath[i])
		}
	}
}

func TestSwitchCompartmentTabForcesSubtreeOn(t *testing.T) {
	m := Model{
		resources: registry.All(nil),
		scope:     registry.Scope{Region: "us-ashburn-1", CompartmentID: "root"},
		table:     newTable(20),
		compTree:  buildTestTree(),
	}
	cmd := m.switchCompartment("a", "a", boolPtr(true))
	if !m.subtreeOn {
		t.Error("subtreeOn = false, want true after a forced-subtree switch")
	}
	if cmd == nil {
		t.Fatal("switchCompartment returned a nil Cmd (expected a fan-out batch)")
	}
	if len(m.recentList) != 1 || !m.recentList[0].Subtree {
		t.Errorf("recentList entry should record Subtree:true, got %v", m.recentList)
	}
}

// TestSwitchResourceWhileSubtreeOnDoesNotPanicOnStaleRows reproduces a
// reported crash: with subtree mode on, switching resource type (e.g.
// Instance -> Subnet via "f"/"tab") used to run the *previous* resource's
// leftover subtree rows through the *new* resource's columns before the
// fan-out reset them — Subnet's Get type-asserts row.Raw.(core.Subnet),
// which panics on anything else (interface conversion).
func TestSwitchResourceWhileSubtreeOnDoesNotPanicOnStaleRows(t *testing.T) {
	resources := registry.All(nil)
	instanceIdx, subnetIdx := -1, -1
	for i, r := range resources {
		switch r.Key() {
		case "instance":
			instanceIdx = i
		case "subnet":
			subnetIdx = i
		}
	}
	if instanceIdx < 0 || subnetIdx < 0 {
		t.Fatal("expected both instance and subnet resources to be registered")
	}

	m := Model{
		resources: resources,
		resIdx:    instanceIdx,
		scope:     registry.Scope{Region: "us-ashburn-1", CompartmentID: "root"},
		table:     newTable(20),
		compTree:  buildTestTree(),
		subtreeOn: true,
		// Leftover state from a completed Instance fan-out. Raw isn't a
		// real instanceRow (unexported outside registry), but any type
		// Subnet's Get doesn't expect reproduces the same panic class.
		subtreeTargets: []subtreeTarget{{ID: "root"}},
		subtreeDone:    map[string]bool{"root": true},
		subtreeRows:    map[string][]registry.Row{"root": {{ID: "inst1", Raw: "not a subnet"}}},
	}
	// Note: no setDisplayRows() call here — the pre-switch state (real
	// instanceRow rows matching the Instance columns already on screen)
	// isn't what's under test; what matters is that switchResource itself,
	// with this leftover subtreeRows content, survives being pointed at
	// Subnet's columns.

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("switchResource panicked switching instance -> subnet while subtree was on: %v", r)
		}
	}()
	_ = m.switchResource(subnetIdx)
}

// TestSwitchCompartmentTurningSubtreeOffDoesNotPanicOnPendingPlaceholder is
// the same panic class as the switchResource case above, triggered instead
// by turning subtree mode off (e.g. a Recent hotkey for a non-subtree
// entry) while a fan-out placeholder ("— loading —", Raw == nil) is still
// on screen — relayout() must not run that leftover row through the
// now-unwrapped, placeholder-unaware columns.
func TestSwitchCompartmentTurningSubtreeOffDoesNotPanicOnPendingPlaceholder(t *testing.T) {
	resources := registry.All(nil)
	subnetIdx := -1
	for i, r := range resources {
		if r.Key() == "subnet" {
			subnetIdx = i
		}
	}
	if subnetIdx < 0 {
		t.Fatal("expected subnet resource to be registered")
	}
	m := Model{
		resources:      resources,
		resIdx:         subnetIdx,
		scope:          registry.Scope{Region: "us-ashburn-1", CompartmentID: "a"},
		table:          newTable(20),
		compTree:       buildTestTree(),
		subtreeOn:      true,
		subtreeTargets: []subtreeTarget{{ID: "a"}, {ID: "b"}},
		subtreeDone:    map[string]bool{"a": true}, // "b" still pending -> a placeholder row
		subtreeRows:    map[string][]registry.Row{"a": {{ID: "s1", Raw: core.Subnet{}}}},
	}
	m.setDisplayRows()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("switchCompartment panicked turning subtree off with a pending placeholder row: %v", r)
		}
	}()
	_ = m.switchCompartment("c", "c", boolPtr(false))
}

// TestSubtreeColumnsPlaceholderRows exercises the placeholder-safe column
// wrapping F3 relies on: a "— loading —" row must never reach a real
// column's Get (which type-asserts Row.Raw and would panic on the nil Raw
// a placeholder carries).
func TestSubtreeColumnsPlaceholderRows(t *testing.T) {
	cols := []registry.Column{
		{Header: "NAME", Get: func(row registry.Row) string { return row.Raw.(string) }},
		{Header: "STATE", Get: func(row registry.Row) string { return row.Raw.(string) }},
	}
	wrapped := subtreeColumns(cols)
	if len(wrapped) != 3 {
		t.Fatalf("len(wrapped) = %d, want 3 (COMPARTMENT + 2 original)", len(wrapped))
	}
	if wrapped[0].Header != "COMPARTMENT" {
		t.Errorf("wrapped[0].Header = %q, want COMPARTMENT", wrapped[0].Header)
	}

	placeholder := registry.Row{ID: subtreePlaceholderPrefix + "x", Name: "— loading —", CompartmentLabel: "db/backup"}
	if got := wrapped[0].Get(placeholder); got != "db/backup" {
		t.Errorf("COMPARTMENT column for placeholder = %q, want %q", got, "db/backup")
	}
	if got := wrapped[1].Get(placeholder); got != "— loading —" {
		t.Errorf("first real column for placeholder = %q, want the loading text", got)
	}
	if got := wrapped[2].Get(placeholder); got != "" {
		t.Errorf("second real column for placeholder = %q, want empty", got)
	}

	real := registry.Row{ID: "ocid1.x", Raw: "hello", CompartmentLabel: "hub-and-spoke"}
	if got := wrapped[1].Get(real); got != "hello" {
		t.Errorf("real row through wrapped column = %q, want %q (should still call the original Get)", got, "hello")
	}
}

func TestSelectedSkipsPlaceholderRow(t *testing.T) {
	m := Model{table: newTable(20)}
	m.displayRows = []registry.Row{{ID: subtreePlaceholderPrefix + "x", Name: "— loading —"}}
	m.table.SetCursor(0)
	if _, ok := m.selected(); ok {
		t.Error("selected() returned ok=true for a placeholder row; actions/ssh/copy would run against a fake ID")
	}
}
