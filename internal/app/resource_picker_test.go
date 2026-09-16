package app

import (
	"context"
	"testing"

	"toci/internal/registry"
)

// fakeResource is a minimal registry.Resource stand-in for testing
// resourcePickerItems' "Other" fallback without touching the real registry.
type fakeResource struct{ key, label string }

func (f fakeResource) Key() string                { return f.key }
func (f fakeResource) Label() string              { return f.label }
func (f fakeResource) Columns() []registry.Column { return nil }
func (f fakeResource) List(context.Context, registry.Scope, string) ([]registry.Row, string, error) {
	return nil, "", nil
}

func TestResourcePickerItemsGroupsByCategory(t *testing.T) {
	resources := registry.All(nil)
	items := resourcePickerItems(resources, "instance", "")

	if got, want := pickerLeafCount(items), len(resources); got != want {
		t.Errorf("pickerLeafCount() = %d, want %d (every resource shown, empty query)", got, want)
	}

	var sawInstanceUnderCompute bool
	category := ""
	for _, it := range items {
		if it.key == "" {
			category = it.label
			continue
		}
		if it.key == "instance" {
			if category != "Compute" {
				t.Errorf("instance filed under category %q, want %q", category, "Compute")
			}
			if !it.isCurrent {
				t.Error("instance should be marked isCurrent (currentKey matched)")
			}
			if it.glyph == "" {
				t.Error("instance should have a tree connector glyph")
			}
			sawInstanceUnderCompute = category == "Compute"
		}
	}
	if !sawInstanceUnderCompute {
		t.Errorf("Compute category with instance not found in items: %+v", items)
	}
}

// TestResourcePickerItemsFuzzyFilterPreservesCategoryOrder guards a subtle
// bug: fuzzy.Find ranks by score, not original position, so filtering
// naively (without re-sorting matches back to their original index) would
// interleave categories together instead of narrowing which whole
// categories show — e.g. querying something that matches resources in
// several categories must still list them in resourceCategories' own
// order, not whichever scored highest.
func TestResourcePickerItemsFuzzyFilterPreservesCategoryOrder(t *testing.T) {
	resources := registry.All(nil)
	// "s" matches something in nearly every category (Subnets, Security
	// Lists, DB Systems, ...) — exercises the reordering risk directly.
	items := resourcePickerItems(resources, "", "s")

	var order []string
	seen := map[string]bool{}
	for _, it := range items {
		if it.key == "" && !seen[it.label] {
			order = append(order, it.label)
			seen[it.label] = true
		}
	}

	wantOrder := []string{"Compute", "Network", "Gateways", "Storage", "Containers", "Database", "Governance"} // resourceCategories' own declared order
	i := 0
	for _, w := range wantOrder {
		if i < len(order) && order[i] == w {
			i++
		}
	}
	if i != len(order) {
		t.Errorf("category order = %v, want a subsequence of %v (categories must not interleave)", order, wantOrder)
	}
}

// TestResourcePickerItemsDoesNotMatchAcrossCategoryAndLabel reproduces a
// reported bug: querying "vcn" matched "Governance/Compartments" — neither
// "Governance" nor "Compartments" alone has anything to do with VCNs, but
// concatenated ("Governance/Compartments") a fuzzy subsequence match for
// "vcn" exists anyway (the "v"/"c" from Go[v]ernan[c]e, the "n" from
// Compart[n]ents). Category and label must be matched separately.
func TestResourcePickerItemsDoesNotMatchAcrossCategoryAndLabel(t *testing.T) {
	resources := registry.All(nil)
	items := resourcePickerItems(resources, "", "vcn")

	for _, it := range items {
		if it.key == "compartment" {
			t.Errorf("querying %q matched Compartments — a cross-boundary false positive: %+v", "vcn", items)
		}
	}
	found := false
	for _, it := range items {
		if it.key == "vcn" {
			found = true
		}
	}
	if !found {
		t.Errorf("querying %q should still match VCNs itself: %+v", "vcn", items)
	}
}

// TestResourcePickerItemsMatchesOCIAbbreviations: users search by the OCI
// abbreviation they already know (e.g. "dbcs" for DB Systems, the reported
// example) rather than toci's own spelled-out label — resourceSearchAliases
// should surface the right resource for each of them.
func TestResourcePickerItemsMatchesOCIAbbreviations(t *testing.T) {
	resources := registry.All(nil)

	cases := []struct{ query, wantKey string }{
		{"dbcs", "db-system"},
		{"sl", "security-list"},
		{"sg", "service-gateway"},
		{"nat", "nat-gateway"},
		{"igw", "igw"},
		{"vm", "instance"},
		{"bm", "instance"},
		{"os", "bucket"},
		{"fss", "file-system"},
		{"adw", "adb"},
		{"atp", "adb"},
		{"exacs", "exadata"},
	}
	for _, c := range cases {
		items := resourcePickerItems(resources, "", c.query)
		found := false
		for _, it := range items {
			if it.key == c.wantKey {
				found = true
			}
		}
		if !found {
			t.Errorf("querying %q should match %q, got %+v", c.query, c.wantKey, items)
		}
	}
}

// TestResourcePickerItemsExcludesMidWordScatteredMatches reproduces two
// reported bugs: querying "dbcs" (meant for DB Systems via its alias) and
// "adb" (meant for Autonomous DB, also via its alias) both surfaced Load
// Balancers too, ahead of the intended match — fuzzy.Find's lenient
// subsequence matching happened to find each query's own letters scattered
// across the *middle* of "Load Balancers" (e.g. "adb" as the "a"/"d" in
// "Lo[a][d]" plus the "B" starting "Balancers"), and results aren't
// reordered by score (see TestResourcePickerItemsFuzzyFilterPreservesCategoryOrder),
// so the accidental hit landed above the real one just by category order.
// matchStartsAtWordBoundary fixes this by requiring a match's first hit to
// land on an actual word start, not partway through one.
func TestResourcePickerItemsExcludesMidWordScatteredMatches(t *testing.T) {
	resources := registry.All(nil)

	cases := []struct{ query, wantKey string }{
		{"dbcs", "db-system"},
		{"adb", "adb"},
	}
	for _, c := range cases {
		items := resourcePickerItems(resources, "", c.query)
		for _, it := range items {
			if it.key == "lb" {
				t.Errorf("querying %q should not match Load Balancers (a mid-word scattered fuzzy hit, not a real one), got %+v", c.query, items)
			}
		}
		found := false
		for _, it := range items {
			if it.key == c.wantKey {
				found = true
			}
		}
		if !found {
			t.Errorf("querying %q should still match %q, got %+v", c.query, c.wantKey, items)
		}
	}
}

// TestResourcePickerItemsMatchesByFirstLetter: typing a single letter
// should surface resources whose name (or a word within it) actually
// starts with that letter, not "no matches" — fuzzy.Find's raw score for
// a lone character is often negative even when it does land on a real
// word start (e.g. "a" against "All Gateways" scores -1), so this must go
// through matchStartsAtWordBoundary rather than a score-based cutoff.
func TestResourcePickerItemsMatchesByFirstLetter(t *testing.T) {
	resources := registry.All(nil)
	items := resourcePickerItems(resources, "", "a")

	for _, wantKey := range []string{"gateway", "adb"} { // "All Gateways", "Autonomous DBs" — both start with "A"
		found := false
		for _, it := range items {
			if it.key == wantKey {
				found = true
			}
		}
		if !found {
			t.Errorf(`querying "a" should match %q (its label starts with "A"), got %+v`, wantKey, items)
		}
	}
}

func TestResourcePickerItemsUnknownResourceFallsBackToOther(t *testing.T) {
	items := resourcePickerItems([]registry.Resource{fakeResource{key: "mystery", label: "Mystery Kind"}}, "", "")

	if len(items) != 2 { // "Other" header + the one resource
		t.Fatalf("items = %+v, want a 2-item [Other header, resource] list", items)
	}
	if items[0].label != "Other" {
		t.Errorf("items[0].label = %q, want %q", items[0].label, "Other")
	}
	if items[1].key != "mystery" {
		t.Errorf("items[1].key = %q, want %q", items[1].key, "mystery")
	}
}
