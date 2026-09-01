package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	// Foreground uses 256-color index 16 (pure black), not basic color "0".
	// Most terminals treat SGR bold + a *basic* 0-7 foreground color as a
	// request for the bright variant (bold historically doubled as an
	// intensity toggle) — so Bold(true) with Color("0") rendered as gray,
	// not black. 256-color codes go through a different SGR form that
	// terminals don't apply that hack to.
	stateBgRunning         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("2"))
	stateBgStopped         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("1"))
	stateBgRunningSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("28")) // dark green
	stateBgStoppedSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("88")) // dark red

	// selectedLinePrefix is the exact ANSI prefix bubbles emits when it
	// wraps a whole row in selStyle (see renderRow in bubbles' table.go).
	// A rendered row starts with this iff it's the cursor row — cheaper
	// and more robust than re-deriving bubbles' internal cursor/viewport
	// offset math ourselves, and it's not exported.
	selectedLinePrefix = func() string {
		probe := selStyle.Render("\x01")
		before, _, found := strings.Cut(probe, "\x01")
		if !found {
			return ""
		}
		return before
	}()
)

var whiteTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

// whitenDataRows recolors every plain (non-header, non-selected) row of an
// already-rendered table view to a bright white foreground — bubbles'
// default Cell style leaves rows uncolored, falling back to the terminal's
// own (often dimmer) default foreground.
//
// This has to run *before* colorizeInstanceState, not after or via
// table.Styles.Cell directly — setting Cell's own Foreground was tried
// first and broke the selected row: bubbles renders every cell
// independently (own open+reset) and only *afterward* wraps the whole row
// in Selected if it's the cursor row, so a per-cell reset baked in ahead of
// time cuts that outer wrap's background off after the first cell. Wrapping
// each full row exactly once here — and skipping the selected line
// entirely, since selStyle already sets it white — avoids that, and gives
// colorizeInstanceState's ansi.Cut splice something already "open" to
// carry forward past the STATE badge on non-selected rows too.
func whitenDataRows(view string) string {
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if i == 0 {
			continue // header row — already styled by table.Styles.Header
		}
		if selectedLinePrefix != "" && strings.HasPrefix(line, selectedLinePrefix) {
			continue // already white (and backgrounded) via selStyle
		}
		lines[i] = whiteTextStyle.Render(line)
	}
	return strings.Join(lines, "\n")
}

// colorizeInstanceState highlights RUNNING/STOPPED in the STATE column of an
// already-rendered Instance table (the string bubbles' table.Model.View()
// returns).
//
// This runs as a post-render pass instead of coloring the cell value at
// Column.Get() time, which was tried first and broke two ways: bubbles
// truncates every cell with go-runewidth BEFORE any styling is applied, and
// go-runewidth counts raw ANSI escape bytes as visible width — so an
// embedded color code gets cut mid-sequence and bleeds color into the rest
// of the table. Using a full ANSI reset after the badge to avoid that also
// broke the selected row's own highlight: the reset can't tell it's inside
// another style's span, so it killed the highlight for every column after
// STATE.
//
// Post-render splicing avoids both: ansi.Cut is ANSI/width-aware (it won't
// truncate mid-escape-code), and critically, the segment it returns for
// "everything after the STATE column" carries forward whatever styling was
// already open at that cut point — the selected row's highlight, if any —
// so the reassembled line keeps that highlight going right through and past
// the colored badge instead of cutting it off.
func colorizeInstanceState(view string, cols []table.Column) string {
	start, end, ok := stateColumnRange(cols)
	if !ok {
		return view
	}

	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if i == 0 {
			continue // header row — nothing to color
		}
		selected := selectedLinePrefix != "" && strings.HasPrefix(line, selectedLinePrefix)
		mid := ansi.Strip(ansi.Cut(line, start, end))
		var style lipgloss.Style
		switch {
		case strings.Contains(mid, "RUNNING"):
			style = stateBgRunning
			if selected {
				style = stateBgRunningSelected
			}
		case strings.Contains(mid, "STOPPED"):
			style = stateBgStopped
			if selected {
				style = stateBgStoppedSelected
			}
		default:
			continue
		}
		// Paint the whole cell (word + its padding), not just the word —
		// coloring only the word left a ring of uncolored blank space
		// around it inside the column.
		left := ansi.Cut(line, 0, start)
		right := ansi.Cut(line, end, 1<<20)
		lines[i] = left + style.Render(mid) + right
	}
	return strings.Join(lines, "\n")
}

// stateColumnRange returns the STATE column's display-column span within a
// rendered table row. Each column occupies col.Width+2 cells on screen —
// bubbles' default Cell/Header style adds 1 space of padding on each side —
// so the offset has to account for that, not just col.Width.
//
// This takes the table's actual rendered columns (m.table.Columns()), not
// registry.Resource.Columns()'s declared widths — those widths are only a
// ceiling now (refreshTable shrinks each column to fit its loaded data), so
// computing offsets from the declared widths pointed past wherever STATE
// actually landed on screen and silently colored nothing.
func stateColumnRange(cols []table.Column) (start, end int, ok bool) {
	offset := 0
	for _, c := range cols {
		span := c.Width + 2
		if c.Title == "STATE" {
			return offset, offset + span, true
		}
		offset += span
	}
	return 0, 0, false
}
