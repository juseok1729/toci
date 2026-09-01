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
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(ociHighlt)).Bold(true)
	helpDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ociSubtle))
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
		BorderForeground(lipgloss.Color(ociBorder)).
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
	boxWidth, boxLines := overlayBoxDims(box)
	baseLines := strings.Split(base, "\n")
	return spliceOverlay(baseLines, boxLines, termWidth-boxWidth, len(baseLines)-len(boxLines))
}

// overlayRightAt splices box right-aligned to termWidth, starting at a
// given row — same splicing as overlayBottomRight, just at an arbitrary
// row instead of pinned to the last one. Used to stack the corner wordmark
// and its version/subtitle line at specific rows of the header.
func overlayRightAt(base, box string, termWidth, row int) string {
	boxWidth, boxLines := overlayBoxDims(box)
	baseLines := strings.Split(base, "\n")
	return spliceOverlay(baseLines, boxLines, termWidth-boxWidth, row)
}

// overlayTopRight splices box onto the top-right corner (row 0). Used for
// the small "toci" wordmark in the corner of the main screen.
func overlayTopRight(base, box string, termWidth int) string {
	return overlayRightAt(base, box, termWidth, 0)
}

// overlayCenter splices box onto an already-rendered view, centered
// horizontally and a third of the way down vertically (rather than dead
// center — reads better above a table that already draws the eye toward
// its top) — same ansi.Cut-based splicing as overlayBottomRight. Used for
// the "f" resource-search picker, which floats over the table rather than
// replacing it.
func overlayCenter(base, box string, termWidth, termHeight int) string {
	boxWidth, boxLines := overlayBoxDims(box)
	baseLines := strings.Split(base, "\n")
	x := (termWidth - boxWidth) / 2
	y := (termHeight - len(boxLines)) / 3
	return spliceOverlay(baseLines, boxLines, x, y)
}

func overlayBoxDims(box string) (width int, lines []string) {
	lines = strings.Split(box, "\n")
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > width {
			width = w
		}
	}
	return width, lines
}

// embedInLine punches label into an already-rendered line at column x,
// keeping the line's original content on both sides — used to set a title
// or a right-aligned count into a box's border line, the same left+mid+right
// splice colorizeInstanceState uses for a table cell.
func embedInLine(line, label string, x int) string {
	w := ansi.StringWidth(label)
	left := ansi.Cut(line, 0, x)
	right := ansi.Cut(line, x+w, 1<<20)
	return left + label + right
}

func spliceOverlay(baseLines, boxLines []string, x, y int) string {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	for i, boxLine := range boxLines {
		row := y + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		left := ansi.Cut(baseLines[row], 0, x)
		// Cut can't manufacture columns that aren't there — a base line
		// shorter than x (e.g. the short "Profile: ..." header line under
		// cornerLogo) comes back as-is, which would land the box right
		// after the short text instead of at column x. Pad it out first.
		if w := ansi.StringWidth(left); w < x {
			left += strings.Repeat(" ", x-w)
		}
		// Keep whatever was past the box's own right edge too — for a
		// corner overlay (help box, cornerLogo) the box already reaches
		// the true edge so this is empty and a no-op, but a narrower,
		// centered box (the "f" resource search) would otherwise wipe out
		// real content to its right, like a table box's own border, on
		// every row it overlaps.
		right := ansi.Cut(baseLines[row], x+ansi.StringWidth(boxLine), 1<<20)
		baseLines[row] = left + boxLine + right
	}
	return strings.Join(baseLines, "\n")
}
