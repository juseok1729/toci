package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// mapNode is one box in the resource map — a subnet, route table, or
// network connection (gateway/DRG). detail is an optional second line
// (a subnet's CIDR, a NAT gateway's IP), rendered in stateTextWarn's
// yellow — the box's normal (unhighlighted) text otherwise inherits the
// terminal's plain default color, which read as too easy to miss. group
// is an optional header shown once above the first item of a new group
// (e.g. an availability domain), "" for no grouping.
type mapNode struct {
	id, label, detail, group string
}

// mapBoxStyle is the resource map's card style — no vertical padding, so
// every box is exactly 3 lines (border/content/border). That keeps row
// bookkeeping between adjacent columns simple: an item's connector row is
// always its box's single content line. highlight picks out the box
// that's part of the currently-selected subnet's path (see
// renderResourceMap) with the same accent color the connector bus uses
// for that path (mapBusHighlightStyle) instead of the normal border color.
//
// A highlighted box colors its *label text* too, not just the border —
// BorderForeground only reaches the border characters, so a highlight
// that set only that (an earlier version of this) left every label the
// terminal's plain default color, bold but otherwise unchanged, which
// read as barely different from a normal box.
//
// width is the *content* width (mapColWidth's answer, sized off label
// lengths) — lipgloss's own Width() sets the box's total rendered width
// including its border and padding, so tableBoxOverhead (this style's
// exact border+padding shape: a 1-column rounded border plus Padding(0,1)
// on each side, 4 columns total) is added back on top of it. Without
// this, a label exactly mapColWidth long would wrap inside its own box —
// wordwrap has no room left after Width() subtracts the overhead again.
func mapBoxStyle(width int, highlight bool) lipgloss.Style {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ociBorder)).
		Padding(0, 1).
		Width(width + tableBoxOverhead)
	if highlight {
		style = style.
			BorderForeground(lipgloss.Color(ociHighlt)).
			Foreground(lipgloss.Color(ociHighlt)).
			Bold(true)
	}
	return style
}

var mapGroupHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ociSubtle)).Bold(true)

// mapBusStyle/mapBusHighlightStyle color a connector bus's plain
// characters (see renderConnectorBus/applyBusHighlight) — the default
// path matches the boxes' own border color, and the highlighted path
// (the selected subnet's route to its gateways) uses the same accent as
// mapBoxStyle's highlighted boxes, so a selection reads as one continuous
// colored line from subnet to gateway.
var (
	mapBusStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color(ociBorder))
	mapBusHighlightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ociHighlt)).Bold(true)
)

// mapColWidth is a column's box width: wide enough for its longest label
// or detail line — measured separately, not the combined rune count of
// "label\ndetail" as one string, which would count the newline and detail
// text as more characters tacked onto the label — capped at ceiling (a
// hard cap, not a hint — matches fitColumnWidth's own doc elsewhere) so
// one long resource name can't blow out the whole map.
func mapColWidth(items []mapNode, ceiling int) int {
	w := 0
	for _, it := range items {
		for _, line := range strings.Split(it.label, "\n") {
			if l := len([]rune(line)); l > w {
				w = l
			}
		}
		if l := len([]rune(it.detail)); l > w {
			w = l
		}
	}
	if w > ceiling {
		w = ceiling
	}
	if w < 8 {
		w = 8
	}
	return w
}

// renderMapColumn lays out one resource-map column's boxes top-aligned,
// one per item plus a blank spacer line between them, with a group header
// line before the first item of each new group. highlight (nil for none)
// marks item IDs to draw in the selected-path accent color instead of the
// normal border (see mapBoxStyle). Returns the rendered lines and each
// item's row within them (its box's single content line — border lines
// sit one above and one below), for connectorEdge to reference.
func renderMapColumn(items []mapNode, width int, highlight map[string]bool) (lines []string, rowOf map[string]int) {
	rowOf = make(map[string]int, len(items))
	lastGroup := ""
	for i, it := range items {
		if i > 0 {
			lines = append(lines, "")
		}
		if it.group != "" && it.group != lastGroup {
			lines = append(lines, mapGroupHeaderStyle.Render(it.group))
			lastGroup = it.group
		}
		hl := highlight[it.id]
		content := it.label
		if it.detail != "" {
			detail := it.detail
			if !hl {
				// Only when *not* highlighted: mapBoxStyle(_, true) sets a
				// whole-content Foreground for the highlighted case, and
				// nesting this pre-styled detail line inside that would
				// leave its own embedded reset code cutting the outer
				// gold styling short partway through the line (the same
				// nested-ANSI hazard embedTwoInLine's own doc warns
				// about) — the highlighted style already makes the whole
				// box maximally visible on its own, detail line included.
				detail = stateTextWarn.Render(detail)
			}
			content += "\n" + detail
		}
		box := strings.Split(mapBoxStyle(width, hl).Render(content), "\n")
		rowOf[it.id] = len(lines) + 1 // box's content line: border, content, border
		lines = append(lines, box...)
	}
	return lines, rowOf
}

// padLines pads lines to height with blank rows — lipgloss.JoinHorizontal
// already does this internally, but the connector bus needs to know the
// shared height up front to size itself, so every block is padded
// explicitly to the same height before composing them.
func padLines(lines []string, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

// connectorEdge is one (row in the left column, row in the right column)
// pair to connect through a shared vertical bus between two adjacent
// resource-map columns.
type connectorEdge struct{ from, to int }

// buildEdges resolves a from-column's items (fromRow: item ID -> row) to
// their target IDs (targets: from-ID -> to-IDs) and finds each target's
// row (toRow), producing one connectorEdge per resolved pair. A target ID
// with no known row — a route rule pointing at something this map doesn't
// track, say — is skipped rather than guessed at.
func buildEdges(fromRow map[string]int, targets map[string][]string, toRow map[string]int) []connectorEdge {
	var edges []connectorEdge
	for fromID, r := range fromRow {
		for _, targetID := range targets[fromID] {
			if tr, ok := toRow[targetID]; ok {
				edges = append(edges, connectorEdge{from: r, to: tr})
			}
		}
	}
	return edges
}

// renderConnectorBus draws the vertical bus between two adjacent resource
// map columns: every edge's row on the left gets a "─" stub reaching a
// shared spine, and every edge's row on the right gets one leaving it — a
// single shared spine rather than computing non-crossing paths per edge,
// so a many-to-one (several subnets, one route table) or one-to-many (one
// route table, several gateways) relationship draws correctly without
// real graph layout. The one case worth a straight line instead of a
// junction glyph is a lone 1:1 edge landing on the same row.
func renderConnectorBus(edges []connectorEdge, height int) []string {
	lines := make([]string, height)
	if len(edges) == 0 {
		for i := range lines {
			lines[i] = "   "
		}
		return lines
	}
	if len(edges) == 1 && edges[0].from == edges[0].to {
		for i := range lines {
			if i == edges[0].from {
				lines[i] = "───"
			} else {
				lines[i] = "   "
			}
		}
		return lines
	}

	fromRows := map[int]bool{}
	toRows := map[int]bool{}
	minRow, maxRow := edges[0].from, edges[0].from
	for _, e := range edges {
		fromRows[e.from] = true
		toRows[e.to] = true
		for _, r := range [2]int{e.from, e.to} {
			minRow = min(minRow, r)
			maxRow = max(maxRow, r)
		}
	}

	for row := 0; row < height; row++ {
		left, right := " ", " "
		if fromRows[row] {
			left = "─"
		}
		if toRows[row] {
			right = "─"
		}
		spine := " "
		if row >= minRow && row <= maxRow {
			switch {
			case fromRows[row] && toRows[row]:
				spine = "┼"
			case fromRows[row]:
				spine = "┤"
			case toRows[row]:
				spine = "├"
			default:
				spine = "│"
			}
		}
		lines[row] = left + spine + right
	}
	return lines
}

// applyBusHighlight colors renderConnectorBus's plain 3-char-per-row
// output — mapBusStyle (matching the boxes' own border color) by default,
// mapBusHighlightStyle for any row a highlighted edge's own [from, to]
// span touches. A row's whole 3-char cell gets one color or the other,
// even on the rare row a highlighted and a non-highlighted edge both
// touch (a gateway shared by the selected route table and another one,
// say) — that row genuinely is part of the selected path, so leaning
// toward highlighting it is the more correct read, and per-character
// mixed styling for that edge case isn't worth the complexity.
func applyBusHighlight(lines []string, highlightEdges []connectorEdge) []string {
	highlightRows := map[int]bool{}
	for _, e := range highlightEdges {
		for r := min(e.from, e.to); r <= max(e.from, e.to); r++ {
			highlightRows[r] = true
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if highlightRows[i] {
			out[i] = mapBusHighlightStyle.Render(l)
		} else {
			out[i] = mapBusStyle.Render(l)
		}
	}
	return out
}

// mapTitleOffset is how many lines a column's title header ("Subnets (6)"
// + a blank separator) takes up before its first box — every column and
// connector bus is prefixed by exactly this many lines, so a box's row
// (from renderMapColumn, 0-based within the boxes only) lands on the same
// absolute line across every column and bus once titles are added.
const mapTitleOffset = 2

// prefixTitle renders a column's title line, styled, plus the blank
// separator every column uses (see mapTitleOffset).
func prefixTitle(text string, count int) []string {
	if count >= 0 {
		text = fmt.Sprintf("%s (%d)", text, count)
	}
	lines := make([]string, mapTitleOffset)
	lines[0] = titleStyle.Render(text)
	return lines
}

// busBlankPrefix is a connector bus's own mapTitleOffset-line header gap —
// buses have no title of their own, just the same blank lines every
// column's title+separator would otherwise occupy.
func busBlankPrefix() []string {
	return make([]string, mapTitleOffset)
}
