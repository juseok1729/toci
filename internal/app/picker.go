package app

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
)

type pickerKind int

const (
	pickerRegion pickerKind = iota
	pickerAction
	pickerBastion
	pickerSSHMode
	pickerSSHKey
	pickerResource
	pickerCompartment
)

type pickerItem struct {
	key   string
	label string

	// The fields below are set only for pickerCompartment items (see
	// compartment_picker.go) — every other picker kind leaves them zero.
	glyph     string // precomputed tree indentation + connector, e.g. "│  ├─ "
	subCount  int    // descendant count, shown as "⊕ n"
	desc      string // compartment description, shown under the highlighted row
	isCurrent bool   // this is the active compartment ("●")
	pinned    bool   // pinned in the MRU list
}

// picker is a fuzzy-filtered list overlay reused for the region switcher,
// the action menu, the bastion picker, the ":" resource search, and the
// "c" compartment tree picker.
type picker struct {
	kind     pickerKind
	title    string
	input    textinput.Model
	items    []pickerItem
	filtered []pickerItem
	cursor   int

	// treeFilter, set only for pickerCompartment, replaces the default
	// flat fuzzy-over-labels matching below: a compartment tree needs to
	// keep a matched node's ancestors visible (so same-named compartments
	// stay distinguishable by path) and carry per-node tree glyphs/badges,
	// none of which a flat []pickerItem fuzzy match can express.
	treeFilter func(query string) []pickerItem
}

func newPicker(kind pickerKind, title string, items []pickerItem) picker {
	ti := textinput.New()
	if kind == pickerResource {
		ti.Placeholder = "Select the resource you want to view..."
		// The ":" search's own cursor color — every other picker (and the
		// resource table's own row cursor, an unrelated selStyle highlight)
		// keeps the textinput package's default.
		styles := ti.Styles()
		styles.Cursor.Color = lipgloss.Color("#f6cbcb")
		ti.SetStyles(styles)
	} else {
		ti.Placeholder = "type to filter..."
	}
	ti.Prompt = "" // renderPicker/renderResourceSearch draw their own "> " prefix
	ti.SetWidth(textInputWidth)
	ti.Focus()
	return picker{kind: kind, title: title, input: ti, items: items, filtered: items}
}

func (p *picker) refilter() {
	q := p.input.Value()
	if p.treeFilter != nil {
		p.filtered = p.treeFilter(q)
	} else if q == "" {
		p.filtered = p.items
	} else {
		labels := make([]string, len(p.items))
		for i, it := range p.items {
			labels[i] = it.label
		}
		matches := fuzzy.Find(q, labels)
		filtered := make([]pickerItem, len(matches))
		for i, m := range matches {
			filtered[i] = p.items[m.Index]
		}
		p.filtered = filtered
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = len(p.filtered) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.cursor = selectableIndex(p.filtered, p.cursor)
}

func (p *picker) selected() (pickerItem, bool) {
	if p.cursor < 0 || p.cursor >= len(p.filtered) {
		return pickerItem{}, false
	}
	return p.filtered[p.cursor], true
}

// selectableIndex finds the nearest selectable item to from (a real
// resource, pickerItem.key != "") — resourcePickerItems is the only
// producer of the key == "" category-header rows this skips, so this is a
// no-op for every other picker kind. Prefers scanning forward from from;
// falls back to scanning backward if there's nothing selectable ahead.
// Returns from unchanged if the list has nothing selectable at all (e.g.
// filtered to zero items), which selected()'s own bounds check handles.
func selectableIndex(items []pickerItem, from int) int {
	if len(items) == 0 {
		return from
	}
	for i := from; i < len(items); i++ {
		if items[i].key != "" {
			return i
		}
	}
	for i := from; i >= 0; i-- {
		if items[i].key != "" {
			return i
		}
	}
	return from
}

// cursorUp/cursorDown move to the nearest selectable item strictly before/
// after the current cursor, skipping any category headers in between —
// unlike selectableIndex (which can land ON the starting index), so
// pressing up/down always moves off a header rather than settling there.
func (p *picker) cursorUp() {
	for i := p.cursor - 1; i >= 0; i-- {
		if p.filtered[i].key != "" {
			p.cursor = i
			return
		}
	}
}

func (p *picker) cursorDown() {
	for i := p.cursor + 1; i < len(p.filtered); i++ {
		if p.filtered[i].key != "" {
			p.cursor = i
			return
		}
	}
}
