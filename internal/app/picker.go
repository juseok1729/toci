package app

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/sahilm/fuzzy"
)

type pickerKind int

const (
	pickerRegion pickerKind = iota
	pickerAction
	pickerBastion
)

type pickerItem struct {
	key   string
	label string
}

// picker is a fuzzy-filtered list overlay reused for both the ":" resource
// switcher and the region switcher.
type picker struct {
	kind     pickerKind
	title    string
	input    textinput.Model
	items    []pickerItem
	filtered []pickerItem
	cursor   int
}

func newPicker(kind pickerKind, title string, items []pickerItem) picker {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.Focus()
	return picker{kind: kind, title: title, input: ti, items: items, filtered: items}
}

func (p *picker) refilter() {
	q := p.input.Value()
	if q == "" {
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
