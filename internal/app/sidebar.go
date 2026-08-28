package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"toci/internal/registry"
)

// sidebarMinContent/sidebarMaxContent bound the sidebar's content width
// (i.e. the lipgloss Width(), excluding the 2-col border + 2-col padding
// added on top). sidebarMinContent matches the tree's own static labels;
// sidebarMaxContent keeps one long compartment name from eating the screen.
const (
	sidebarMinContent = 20
	sidebarMaxContent = 60

	// mainMinWidth is how much room sidebarContentWidth *prefers* to leave
	// for the table/detail pane before it starts shrinking the sidebar
	// below its natural width — not a hard floor on the main pane itself
	// (that's mainAbsFloor in model.go). On a genuinely narrow terminal,
	// sidebarMinContent + mainMinWidth can add up to more than the whole
	// terminal; sidebarAbsFloor is what keeps that from overflowing.
	mainMinWidth = 30

	// sidebarAbsFloor is the sidebar's true minimum — used instead of
	// sidebarMinContent once the terminal is too narrow to honor the
	// preferred mainMinWidth reservation, so the two sides' floors can't
	// add up to more than the terminal actually has.
	sidebarAbsFloor = 8
)

// resourceCategories groups resource kinds under a tree heading for the
// sidebar. Any key not present in the live resource list is skipped, so this
// stays in sync with registry.All without needing a parallel data model.
//
// vcnScoped marks the category whose resources live "inside" a VCN (either
// natively filterable by VcnId, or joined against one client-side — see
// registry.Scope.VcnID and instance_vcn_filter.go). Picking a VCN row ("i")
// scopes every resource in this category until a non-VCN-scoped one is
// picked; isVcnDependent below is the single source of truth other code
// checks against.
var resourceCategories = []struct {
	label     string
	keys      []string
	vcnScoped bool
}{
	{"Compartments", []string{"compartment"}, false},
	{"VCN-scoped", []string{"vcn", "subnet", "route-table", "security-list", "nsg", "instance", "lb", "db-system", "adb", "exadata"}, true},
	{"Global-scoped", []string{"drg"}, false},
}

// isVcnDependent reports whether switching to this resource should keep an
// active VCN filter (scope.VcnID) instead of clearing it.
func isVcnDependent(key string) bool {
	for _, c := range resourceCategories {
		if !c.vcnScoped {
			continue
		}
		for _, k := range c.keys {
			if k == key {
				return true
			}
		}
	}
	return false
}

type sidebarLeaf struct {
	resIdx int
	label  string
}

type sidebarGroup struct {
	label     string
	vcnScoped bool
	leaves    []sidebarLeaf
}

func buildSidebar(resources []registry.Resource) []sidebarGroup {
	byKey := make(map[string]int, len(resources))
	for i, r := range resources {
		byKey[r.Key()] = i
	}
	var groups []sidebarGroup
	for _, c := range resourceCategories {
		g := sidebarGroup{label: c.label, vcnScoped: c.vcnScoped}
		for _, k := range c.keys {
			if idx, ok := byKey[k]; ok {
				g.leaves = append(g.leaves, sidebarLeaf{resIdx: idx, label: resources[idx].Label()})
			}
		}
		if len(g.leaves) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}

func flatLeaves(groups []sidebarGroup) []sidebarLeaf {
	var out []sidebarLeaf
	for _, g := range groups {
		out = append(out, g.leaves...)
	}
	return out
}

// sidebarLine is one row of the tree, pre-styled, kept separate from the
// text so sidebarContentWidth can measure the plain text (styling doesn't
// change display width, but keeping them apart avoids re-deriving styles
// from ANSI-wrapped strings).
type sidebarLine struct {
	text  string
	style lipgloss.Style
}

func (m Model) sidebarLines() []sidebarLine {
	groups := buildSidebar(m.resources)
	focused := m.mode == modeSidebar

	lines := []sidebarLine{{"Resources", titleStyle}}

	li := 0
	for gi, g := range groups {
		lines = append(lines, sidebarLine{g.label, pathStyle})
		if g.vcnScoped {
			name := m.vcnFilterName
			style := pathStyle
			if name == "" {
				name = "None"
			} else {
				style = titleStyle
			}
			lines = append(lines, sidebarLine{" : " + name, style})
		}
		for i, leaf := range g.leaves {
			branch := "├─"
			if i == len(g.leaves)-1 {
				branch = "└─"
			}
			text := " " + branch + " " + leaf.label
			style := lipgloss.NewStyle()
			switch {
			case focused && li == m.sidebarCursor:
				style = selStyle
			case leaf.resIdx == m.resIdx:
				style = titleStyle
			}
			lines = append(lines, sidebarLine{text, style})
			li++

			if m.resources[leaf.resIdx].Key() == "compartment" {
				base := "    "
				if i < len(g.leaves)-1 {
					base = " │  "
				}
				for ci, c := range m.compPath {
					text := base + strings.Repeat("   ", ci) + "└─ " + c.Name
					st := pathStyle
					if ci == len(m.compPath)-1 {
						st = titleStyle
					}
					lines = append(lines, sidebarLine{text, st})
				}
			}
		}
		if gi < len(groups)-1 {
			lines = append(lines, sidebarLine{"", lipgloss.NewStyle()})
		}
	}
	return lines
}

// sidebarContentWidth is how wide the tree's longest line actually needs —
// mainly driven by long compartment names, which used to wrap inside a
// fixed-width box. Clamped to sidebarMaxContent, and shrunk further only if
// the terminal is too narrow to also leave mainMinWidth for the table pane
// — NOT to half the terminal, which cut long names short on anything but a
// wide terminal (the whole point of sizing this dynamically was to stop
// that wrapping).
func sidebarContentWidth(m Model) int {
	w := sidebarMinContent
	for _, l := range m.sidebarLines() {
		if lw := lipgloss.Width(l.text); lw > w {
			w = lw
		}
	}
	if w > sidebarMaxContent {
		w = sidebarMaxContent
	}
	avail := m.width - 4 - 2 - mainMinWidth
	if avail < sidebarAbsFloor {
		avail = sidebarAbsFloor
	}
	if w > avail {
		w = avail
	}
	return w
}

// sidebarTotalWidth is the sidebar's full on-screen width: content plus the
// 2-col border and 2-col padding renderSidebar adds around it.
func sidebarTotalWidth(m Model) int {
	if m.sidebarHidden {
		return 0
	}
	return sidebarContentWidth(m) + 4
}

func (m Model) renderSidebar() string {
	if m.sidebarHidden {
		return ""
	}

	var b strings.Builder
	for _, l := range m.sidebarLines() {
		b.WriteString(l.style.Render(l.text))
		b.WriteString("\n")
	}

	height := m.height - 4
	if height < 1 {
		height = 1
	}
	// lipgloss's Width() is the box's content+padding width — it wraps text
	// at width-padding, then pads back out to width. Passing the raw text
	// width here would make it wrap 2 cols short (the padding), which was
	// exactly the bug: long compartment names clipped a couple chars in.
	style := lipgloss.NewStyle().
		Width(sidebarContentWidth(m)+2).
		Height(height).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))
	return style.Render(b.String())
}

func (m Model) updateSidebar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	leaves := flatLeaves(buildSidebar(m.resources))
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeTable
		return m, nil
	case "up", "k":
		if m.sidebarCursor > 0 {
			m.sidebarCursor--
		}
		return m, nil
	case "down", "j":
		if m.sidebarCursor < len(leaves)-1 {
			m.sidebarCursor++
		}
		return m, nil
	case "enter":
		m.mode = modeTable
		if m.sidebarCursor < 0 || m.sidebarCursor >= len(leaves) {
			return m, nil
		}
		leaf := leaves[m.sidebarCursor]
		if m.resources[leaf.resIdx].Key() == "compartment" {
			return m, m.switchToRootCompartments()
		}
		return m, m.switchResource(leaf.resIdx)
	}
	return m, nil
}
