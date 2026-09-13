package app

import (
	"sort"

	"github.com/sahilm/fuzzy"

	"toci/internal/registry"
)

// resourceCategories groups registry resource kinds under a heading for
// the "f"/":" resource search's tree picker — the same shape OCI's own
// console groups its left nav into. Order here is both the category
// display order and, within a category, the resource display order.
var resourceCategories = []struct {
	name string
	keys []string
}{
	{"Governance", []string{"compartment"}},
	{"Compute", []string{"instance"}},
	{"Network", []string{
		"vcn", "subnet", "route-table", "security-list", "nsg",
		"drg", "drg-attachment", "drg-route-table", "drg-route-distribution",
		"lb",
	}},
	{"Database", []string{"db-system", "adb", "exadata", "exascale"}},
}

// resourcePickerItems flattens resources into the tree picker's rows:
// a category header (key "") followed by each of its resources, connector
// glyphs included. currentKey marks the active resource with "●". With a
// non-empty query, only resources whose "Category/Label" fuzzy-matches are
// kept — since the category name is baked into that match string, typing
// a category name (e.g. "network") surfaces every resource under it, and
// a resource name still keeps its own category header for context.
//
// Any resource key registry.All() defines but resourceCategories doesn't
// mention (a new resource kind someone forgot to categorize here) is filed
// under "Other" rather than silently disappearing from the picker.
func resourcePickerItems(resources []registry.Resource, currentKey, query string) []pickerItem {
	byKey := make(map[string]registry.Resource, len(resources))
	for _, r := range resources {
		byKey[r.Key()] = r
	}

	type leaf struct {
		category string
		res      registry.Resource
	}
	var leaves []leaf
	seen := make(map[string]bool, len(resources))
	for _, cat := range resourceCategories {
		for _, key := range cat.keys {
			if r, ok := byKey[key]; ok {
				leaves = append(leaves, leaf{cat.name, r})
				seen[key] = true
			}
		}
	}
	for _, r := range resources {
		if !seen[r.Key()] {
			leaves = append(leaves, leaf{"Other", r})
		}
	}

	kept := leaves
	if query != "" {
		paths := make([]string, len(leaves))
		for i, l := range leaves {
			paths[i] = l.category + "/" + l.res.Label()
		}
		matches := fuzzy.Find(query, paths)
		// fuzzy.Find ranks by score, not original position — sort back to
		// original order first, or a query would scramble categories
		// together (e.g. interleave Network and Database rows) instead of
		// just narrowing which ones show.
		idx := make([]int, len(matches))
		for i, mm := range matches {
			idx[i] = mm.Index
		}
		sort.Ints(idx)
		kept = make([]leaf, len(idx))
		for i, li := range idx {
			kept[i] = leaves[li]
		}
	}

	var items []pickerItem
	for i := 0; i < len(kept); {
		category := kept[i].category
		j := i
		for j < len(kept) && kept[j].category == category {
			j++
		}
		items = append(items, pickerItem{label: category})
		for k := i; k < j; k++ {
			connector := "├─ "
			if k == j-1 {
				connector = "└─ "
			}
			r := kept[k].res
			items = append(items, pickerItem{
				key:       r.Key(),
				label:     r.Label(),
				glyph:     connector,
				isCurrent: r.Key() == currentKey,
			})
		}
		i = j
	}
	return items
}

// pickerLeafCount counts the selectable (non-category-header) items in a
// resourcePickerItems result, for the "n/total" count in the picker's
// title bar.
func pickerLeafCount(items []pickerItem) int {
	n := 0
	for _, it := range items {
		if it.key != "" {
			n++
		}
	}
	return n
}
