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

	wantOrder := []string{"Governance", "Compute", "Network", "Database"} // resourceCategories' own declared order
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
