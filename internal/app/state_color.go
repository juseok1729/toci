package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	// stateText{Good,Bad,Warn} color just the STATE value's text — no
	// background — so a normal row's STATE cell blends into the table like
	// any other column instead of reading as a badge. Applies across every
	// resource kind (not just Instance): each kind's LifecycleState values
	// differ, but all of them fall into one of these three buckets — see
	// stateGoodWords/stateBadWords/stateWarnWords below for which words
	// land where, and docs/COLOR_SYSTEM.md for the full per-resource table.
	//
	// The Selected variants additionally set Background to ociSelBg (the
	// same color selStyle highlights the cursor row with): without it,
	// style.Render's own trailing reset would punch a default-background
	// hole in the middle of an otherwise-highlighted selected row right
	// where the STATE text sits (see colorizeState's doc for why the reset
	// does that).
	stateTextGood         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2")) // green
	stateTextBad          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")) // red
	stateTextWarn         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")) // yellow
	stateTextGoodSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2")).Background(lipgloss.Color(ociSelBg))
	stateTextBadSelected  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Background(lipgloss.Color(ociSelBg))
	stateTextWarnSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")).Background(lipgloss.Color(ociSelBg))

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

// blinkStyle is the alternating highlight for rows created/modified within
// the last recentChangesWindow — reuses the gold accent so it reads as
// "look here" without clashing with the STATE text's red/green.
var blinkStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color(ociHighlt))

// blinkRecentRows re-colors (whole row) every row whose NAME matches one of
// recentNames, but only when blinkOn is true — the caller alternates blinkOn
// on a timer, so a row genuinely blinks between this highlight and its
// normal appearance rather than getting stuck highlighted.
//
// Matching by the rendered NAME column's text is the same technique
// colorizeState uses for STATE values: bubbles' viewport/scroll math
// isn't exported, so there's no way to map a rendered line back to its
// registry.Row directly, but every row's NAME is right there in the
// string. Two rows sharing a display name would both blink or neither — an
// accepted edge case.
func blinkRecentRows(view string, cols []table.Column, recentNames map[string]bool, blinkOn bool) string {
	if !blinkOn || len(recentNames) == 0 {
		return view
	}
	start, end, ok := columnRange(cols, "NAME")
	if !ok {
		return view
	}
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		name := strings.TrimSpace(ansi.Strip(ansi.Cut(line, start, end)))
		if !recentNames[name] {
			continue
		}
		lines[i] = blinkStyle.Render(ansi.Strip(line))
	}
	return strings.Join(lines, "\n")
}

// whitenDataRows recolors every plain (non-header, non-selected) row of an
// already-rendered table view to a bright white foreground — bubbles'
// default Cell style leaves rows uncolored, falling back to the terminal's
// own (often dimmer) default foreground.
//
// This has to run *before* colorizeState, not after or via table.Styles.Cell
// directly — setting Cell's own Foreground was tried first and broke the
// selected row: bubbles renders every cell independently (own open+reset)
// and only *afterward* wraps the whole row in Selected if it's the cursor
// row, so a per-cell reset baked in ahead of time cuts that outer wrap's
// background off after the first cell. Wrapping each full row exactly once
// here — and skipping the selected line entirely, since selStyle already
// sets it white — avoids that, and gives colorizeState's ansi.Cut splice
// something already "open" to carry forward past the STATE text on
// non-selected rows too.
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

// stateWarnWords/stateBadWords/stateGoodWords classify a resource's
// (already stateLabel-formatted, e.g. "Needs Attention") STATE text into
// one of three color tiers. Checked in this priority order — Warn, then
// Bad, then Good — because some display strings share a word across tiers
// (ADB's "Available Needs Attention" contains "Available", which alone
// would mean Good; Warn must win there). See docs/COLOR_SYSTEM.md for the
// full per-resource state → tier table.
var (
	stateWarnWords = []string{"Needs Attention"}
	stateBadWords  = []string{"Failed", "Inaccessible", "Unavailable", "Stopped"}
	stateGoodWords = []string{"Running", "Active", "Available", "Standby"}
)

func containsAnyWord(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// stateStyleFor returns the style for a STATE cell's text, and whether it
// matched any tier at all — a transitional value (e.g. "Provisioning",
// "Terminated", "Maintenance In Progress") matches none and is left
// uncolored.
func stateStyleFor(value string, selected bool) (lipgloss.Style, bool) {
	switch {
	case containsAnyWord(value, stateWarnWords):
		if selected {
			return stateTextWarnSelected, true
		}
		return stateTextWarn, true
	case containsAnyWord(value, stateBadWords):
		if selected {
			return stateTextBadSelected, true
		}
		return stateTextBad, true
	case containsAnyWord(value, stateGoodWords):
		if selected {
			return stateTextGoodSelected, true
		}
		return stateTextGood, true
	default:
		return lipgloss.Style{}, false
	}
}

// colorizeState highlights a named column (e.g. "STATE", or DB System's
// "NODE") of an already-rendered resource table (the string bubbles'
// table.Model.View() returns) — every resource kind, not just Instance,
// since every kind's state values end up in one of the three tiers above.
//
// A column like NODE can hold several states joined by "/" (a 2-node RAC
// DB system might show "Available/Stopped") — each "/"-separated part is
// colored independently by its own tier, so that example renders as
// green/red rather than one worst-tier color for the whole cell, letting a
// user tell which specific node needs attention.
//
// This runs as a post-render pass instead of coloring the cell value at
// Column.Get() time, which was tried first and broke two ways: bubbles
// truncates every cell with go-runewidth BEFORE any styling is applied, and
// go-runewidth counts raw ANSI escape bytes as visible width — so an
// embedded color code gets cut mid-sequence and bleeds color into the rest
// of the table. Using a full ANSI reset after the value to avoid that also
// broke the selected row's own highlight: the reset can't tell it's inside
// another style's span, so it killed the highlight for every column after
// STATE.
//
// Post-render splicing avoids both: ansi.Cut is ANSI/width-aware (it won't
// truncate mid-escape-code), and critically, the segment it returns for
// "everything after the STATE column" carries forward whatever styling was
// already open at that cut point — the selected row's highlight, if any —
// so the reassembled line keeps that highlight going right through and past
// the colored text instead of cutting it off.
func colorizeState(view string, cols []table.Column, title string) string {
	start, end, ok := columnRange(cols, title)
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
		recolored, ok := recolorStateCell(mid, selected)
		if !ok {
			continue
		}
		left := ansi.Cut(line, 0, start)
		right := ansi.Cut(line, end, 1<<20)
		lines[i] = left + recolored + right
	}
	return strings.Join(lines, "\n")
}

// recolorStateCell colors a rendered STATE/NODE cell's plain text, one
// "/"-separated part at a time (a single-value STATE cell is just one
// part). Reports false — leaving the caller's line untouched — if no part
// matched any tier.
//
// Each part is colored via its own Render() call, which independently
// opens and resets — so on a selected row, the "/" separators between
// parts need their own Background(ociSelBg) (via selStyle) or they'd show
// up as small unhighlighted gaps in the row's highlight, the same class of
// bug the whole-cell Selected styles exist to avoid.
func recolorStateCell(mid string, selected bool) (string, bool) {
	parts := strings.Split(mid, "/")
	rendered := make([]string, len(parts))
	matched := false
	for i, part := range parts {
		style, ok := stateStyleFor(part, selected)
		if !ok {
			rendered[i] = part
			continue
		}
		matched = true
		rendered[i] = style.Render(part)
	}
	if !matched {
		return "", false
	}
	sep := "/"
	if selected {
		sep = selStyle.Render("/")
	}
	return strings.Join(rendered, sep), true
}

// columnRange returns a named column's display-column span within a
// rendered table row. Each column occupies col.Width+2 cells on screen —
// bubbles' default Cell/Header style adds 1 space of padding on each side —
// so the offset has to account for that, not just col.Width.
//
// This takes the table's actual rendered columns (m.table.Columns()), not
// registry.Resource.Columns()'s declared widths — those widths are only a
// ceiling now (refreshTable shrinks each column to fit its loaded data), so
// computing offsets from the declared widths pointed past wherever STATE
// actually landed on screen and silently colored nothing.
func columnRange(cols []table.Column, title string) (start, end int, ok bool) {
	offset := 0
	for _, c := range cols {
		span := c.Width + 2
		if c.Title == title {
			return offset, offset + span, true
		}
		offset += span
	}
	return 0, 0, false
}
