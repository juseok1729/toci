package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type helpEntry struct {
	key  string
	desc string
}

var (
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	helpDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
)

// renderHelpBox builds the LazyVim-style which-key popup — every
// applicable keybinding (m.helpEntries) as a bordered, right-aligned-key
// list, meant to be overlaid on the bottom-right corner of the screen via
// overlayBottomRight.
func renderHelpBox(m Model) string {
	entries := m.helpEntries()

	keyWidth := 0
	for _, e := range entries {
		if w := lipgloss.Width(e.key); w > keyWidth {
			keyWidth = w
		}
	}

	var b strings.Builder
	for i, e := range entries {
		b.WriteString(helpKeyStyle.Render(fmt.Sprintf("%-*s", keyWidth, e.key)))
		b.WriteString("  ")
		b.WriteString(helpDescStyle.Render(e.desc))
		if i < len(entries)-1 {
			b.WriteString("\n")
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(b.String())
}

// overlayBottomRight splices box onto the bottom-right corner of an
// already-rendered view, replacing whatever base content was there rather
// than appending — the same ansi.Cut-based splicing state_color.go uses to
// recolor one column of a table row, just applied over a rectangular
// region instead of a single line's column range. termWidth is the full
// terminal width box's right edge should align to.
func overlayBottomRight(base, box string, termWidth int) string {
	boxLines := strings.Split(box, "\n")
	boxWidth := 0
	for _, l := range boxLines {
		if w := ansi.StringWidth(l); w > boxWidth {
			boxWidth = w
		}
	}
	x := termWidth - boxWidth
	if x < 0 {
		x = 0
	}

	baseLines := strings.Split(base, "\n")
	startRow := len(baseLines) - len(boxLines)
	if startRow < 0 {
		startRow = 0
	}

	for i, boxLine := range boxLines {
		row := startRow + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		left := ansi.Cut(baseLines[row], 0, x)
		baseLines[row] = left + boxLine
	}
	return strings.Join(baseLines, "\n")
}
