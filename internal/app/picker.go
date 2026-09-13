package app

import (
	"charm.land/bubbles/v2/textinput"
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
// the action menu, the bastion picker, the "f" resource search, and the
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
	ti.Placeholder = "type to filter..."
	ti.Prompt = "" // renderPicker/renderResourceSearch draw their own "> " prefix
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
}

func (p *picker) selected() (pickerItem, bool) {
	if p.cursor < 0 || p.cursor >= len(p.filtered) {
		return pickerItem{}, false
	}
	return p.filtered[p.cursor], true
}
