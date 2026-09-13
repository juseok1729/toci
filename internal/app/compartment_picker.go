package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

// newCompartmentPicker builds the F5 tree picker: a fuzzy-filtered,
// hierarchy-preserving view of the whole cached compartment tree.
// currentID marks the "●" row (the compartment the app is scoped to right
// now); preselectID (often the same value, but see F6's "open on this row")
// is the node the cursor should start on.
func (m *Model) newCompartmentPicker(currentID, preselectID string) picker {
	p := newPicker(pickerCompartment, "Compartments", nil)
	p.treeFilter = func(query string) []pickerItem {
		return compartmentPickerItems(m.compTree, query, m.recentList, currentID)
	}
	p.refilter()
	for i, it := range p.filtered {
		if it.key == preselectID {
			p.cursor = i
			break
		}
	}
	return p
}

// compartmentPickerItems flattens tree into picker rows in tree order,
// each carrying its own precomputed indentation/connector glyph. With a
// non-empty query, only nodes whose name or full path fuzzy-matches (plus
// their ancestors, so path context never disappears) are kept — the tree
// glyphs are still computed against the *filtered* view, so connectors
// never point at a hidden sibling.
func compartmentPickerItems(tree *compartmentTree, query string, recent []recentEntry, currentID string) []pickerItem {
	if tree == nil || tree.root == nil {
		return nil
	}

	keep := map[string]bool{}
	for id := range tree.byID {
		keep[id] = true
	}
	if query != "" {
		type flatNode struct {
			node *compartmentNode
			path string
		}
		var all []flatNode
		var collect func(n *compartmentNode, prefix string)
		collect = func(n *compartmentNode, prefix string) {
			path := n.Name
			if prefix != "" {
				path = prefix + "/" + n.Name
			}
			all = append(all, flatNode{node: n, path: path})
			for _, c := range n.Children {
				collect(c, path)
			}
		}
		collect(tree.root, "")

		paths := make([]string, len(all))
		for i, f := range all {
			paths[i] = f.path
		}
		keep = map[string]bool{}
		for _, mm := range fuzzy.Find(query, paths) {
			for cur := all[mm.Index].node; cur != nil; cur = tree.node(cur.ParentID) {
				keep[cur.ID] = true
			}
		}
	}

	pinned := map[string]bool{}
	for _, e := range recent {
		if e.Pinned {
			pinned[e.ID] = true
		}
	}

	var items []pickerItem
	var walk func(n *compartmentNode, linePrefix, childBasePrefix string)
	walk = func(n *compartmentNode, linePrefix, childBasePrefix string) {
		items = append(items, pickerItem{
			key:       n.ID,
			label:     n.Name,
			glyph:     linePrefix,
			subCount:  n.subtreeCount(),
			desc:      n.Description,
			isCurrent: n.ID == currentID,
			pinned:    pinned[n.ID],
		})
		var kids []*compartmentNode
		for _, c := range n.Children {
			if keep[c.ID] {
				kids = append(kids, c)
			}
		}
		for i, c := range kids {
			last := i == len(kids)-1
			connector, cont := "├─ ", "│  "
			if last {
				connector, cont = "└─ ", "   "
			}
			walk(c, childBasePrefix+connector, childBasePrefix+cont)
		}
	}
	walk(tree.root, "", "")
	return items
}

// renderCompartmentPicker draws the F5 picker: same wide, telescope-style
// frame as renderResourceSearch (title+match-count punched into the top
// border, "> " fuzzy input, divider), but with tree rows and a description
// footer line for the highlighted compartment instead of a flat list.
func (m Model) renderCompartmentPicker() string {
	width := m.width * 3 / 5
	if width < 60 {
		width = 60
	}
	if max := m.width - 4; width > max {
		width = max
	}
	if width < 20 {
		width = 20
	}
	innerWidth := width - 4 // border + padding on both sides, see renderResourceSearch

	var b strings.Builder
	b.WriteString("> ")
	b.WriteString(m.picker.input.View())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", innerWidth))
	b.WriteString("\n")

	visible := m.compartmentPickerVisibleRows()
	start := 0
	if m.picker.cursor >= visible {
		start = m.picker.cursor - visible + 1
	}
	end := start + visible
	if end > len(m.picker.filtered) {
		end = len(m.picker.filtered)
	}

	var selectedDesc string
	for i := start; i < end; i++ {
		it := m.picker.filtered[i]
		line := it.glyph + it.label
		if it.isCurrent {
			line += " ●"
		}
		badge := ""
		if it.subCount > 0 {
			badge = fmt.Sprintf(" ⊕ %d", it.subCount)
		}
		if it.pinned {
			badge += " 📌"
		}
		if i == m.picker.cursor {
			selectedDesc = it.desc
			pad := innerWidth - ansi.StringWidth(line+badge) - 2
			if pad < 0 {
				pad = 0
			}
			b.WriteString(selStyle.Render("› " + line + strings.Repeat(" ", pad) + badge))
		} else {
			pad := innerWidth - ansi.StringWidth(line+badge) - 2
			if pad < 0 {
				pad = 0
			}
			b.WriteString("  " + line + strings.Repeat(" ", pad) + statusStyle.Render(badge))
		}
		b.WriteString("\n")
	}
	if len(m.picker.filtered) == 0 {
		b.WriteString(statusStyle.Render("  (no matches)"))
		b.WriteString("\n")
	}

	b.WriteString(strings.Repeat("─", innerWidth))
	b.WriteString("\n")
	if selectedDesc != "" {
		b.WriteString(statusStyle.Render(truncateWidth(selectedDesc, innerWidth)))
	}
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("enter select · tab +subtree · p pin · esc"))

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ociBorder)).
		Width(width).
		Padding(0, 1)
	lines := strings.Split(style.Render(strings.TrimRight(b.String(), "\n")), "\n")

	title := titleStyle.Render(" Compartments ")
	topWidth := ansi.StringWidth(lines[0])
	titleX := (topWidth - ansi.StringWidth(title)) / 2

	total := 0
	if m.compTree != nil {
		total = len(m.compTree.byID)
	}
	count := statusStyle.Render(fmt.Sprintf(" %d/%d ", len(m.picker.filtered), total))
	countX := topWidth - ansi.StringWidth(count) - 1
	if countX > titleX+ansi.StringWidth(title) {
		lines[0] = embedTwoInLine(lines[0], title, titleX, count, countX)
	} else {
		lines[0] = embedInLine(lines[0], title, titleX)
	}
	return strings.Join(lines, "\n")
}

// compartmentPickerVisibleRows caps the tree list to a sane height rather
// than letting a large tenancy overflow the terminal — same idea as the
// resource table's own height budget.
func (m Model) compartmentPickerVisibleRows() int {
	rows := m.height - 14
	if rows < 5 {
		rows = 5
	}
	if rows > 20 {
		rows = 20
	}
	return rows
}

// truncateWidth clips s to at most w display columns, appending "…" if it
// had to cut.
func truncateWidth(s string, w int) string {
	if ansi.StringWidth(s) <= w || w <= 1 {
		return ansi.Cut(s, 0, w)
	}
	return ansi.Cut(s, 0, w-1) + "…"
}
