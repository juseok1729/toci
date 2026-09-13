package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
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
	return spliceOverlay(baseLines, boxLines, termWidth-boxWidth, len(baseLines)-len(boxLines), termWidth)
}

// overlayRightAt splices box right-aligned to termWidth, starting at a
// given row — same splicing as overlayBottomRight, just at an arbitrary
// row instead of pinned to the last one. Used to stack the corner wordmark
// and its version/subtitle line at specific rows of the header.
func overlayRightAt(base, box string, termWidth, row int) string {
	boxWidth, boxLines := overlayBoxDims(box)
	baseLines := strings.Split(base, "\n")
	return spliceOverlay(baseLines, boxLines, termWidth-boxWidth, row, termWidth)
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
	return spliceOverlay(baseLines, boxLines, x, y, termWidth)
}

// overlayBottom splices box onto the bottom of an already-rendered view,
// horizontally centered — the same idea as overlayBottomRight, just
// centered instead of right-aligned. Used for the "M" resource map, which
// floats over the table (like "f"'s overlayCenter) rather than replacing
// the whole screen the way modeDetail normally does.
func overlayBottom(base, box string, termWidth int) string {
	boxWidth, boxLines := overlayBoxDims(box)
	baseLines := strings.Split(base, "\n")
	x := (termWidth - boxWidth) / 2
	return spliceOverlay(baseLines, boxLines, x, len(baseLines)-len(boxLines), termWidth)
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
// splice colorizeState uses for a table cell.
func embedInLine(line, label string, x int) string {
	w := ansi.StringWidth(label)
	left := ansi.Cut(line, 0, x)
	right := ansi.Cut(line, x+w, 1<<20)
	return left + label + right
}

// embedTwoInLine punches two non-overlapping labels (a before b) into an
// already-rendered line in a single pass — used by renderResourceSearch for
// its title and match-count, which share one border line. Two sequential
// embedInLine calls would work out to the same visible text, but each cut
// re-opens the line's border-color styling at its own boundary, so calling
// it twice leaves a pair of zero-width "open immediately followed by
// reset" style segments sitting right where the count was spliced in —
// harmless in principle, but exactly the kind of degenerate ANSI a real
// terminal's redraw path has no reason to have been exercised against,
// which is what was blanking the dashes around the match count. One cut
// per boundary avoids ever re-slicing already-spliced output.
func embedTwoInLine(line, a string, ax int, b string, bx int) string {
	aw := ansi.StringWidth(a)
	left := ansi.Cut(line, 0, ax)
	mid := ansi.Cut(line, ax+aw, bx)
	right := ansi.Cut(line, bx+ansi.StringWidth(b), 1<<20)
	return left + a + mid + b + right
}

func spliceOverlay(baseLines, boxLines []string, x, y, termWidth int) string {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	// A short base (e.g. the one-line "error: ..."/"loading..." states,
	// nowhere near termHeight) used to make overlayCenter's row math land
	// past the end of baseLines — every boxLine row hit the "continue"
	// below and the whole popup silently vanished. Pad up front instead,
	// so a box always has somewhere to land regardless of how tall base is.
	for needed := y + len(boxLines); len(baseLines) < needed; {
		baseLines = append(baseLines, "")
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
		spliced := left + boxLine + right
		// Belt-and-suspenders: never hand the terminal a row wider than the
		// screen. left/boxLine/right's widths are each measured and cut
		// independently, so any width miscount between them (a mismatched
		// wcwidth table between what built boxLine and what measures it
		// here, say) would otherwise overshoot the terminal's real column
		// count and trigger a hard line-wrap on some terminals — clip back
		// to termWidth rather than trust the arithmetic to always land
		// exactly on it.
		if w := ansi.StringWidth(spliced); w > termWidth {
			spliced = ansi.Cut(spliced, 0, termWidth)
		}
		baseLines[row] = spliced
	}
	return strings.Join(baseLines, "\n")
}
