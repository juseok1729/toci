package app

import (
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// subnetTypeColorPrivate/Selected color the Subnet table's "Private" TYPE
// value — "Public" reuses stateTextGood/stateTextGoodSelected directly
// (state_color.go), the same green "Running"/"Available" already use, per
// explicit request. Private has no existing blue anywhere in this app, so
// this is a new pair, following the same shape as every other per-cell
// color in this package: bold, with a background on the selected variant
// so the row's own selStyle highlight doesn't get punched through by this
// style's Render() (see state_color.go's colorizeState doc for why).
var (
	subnetTypeColorPrivate         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	subnetTypeColorPrivateSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Background(lipgloss.Color(ociSelBg))
)

// subnetTypeStyleFor maps the Subnet table's TYPE cell text (see
// registry.subnetAccessLabel: "Public"/"Private") to its color. Matching
// on the exact cell text — not just the "TYPE" column title, which
// DRG Attachments and DRG Route Distributions also use for unrelated
// values — is what keeps colorizeSubnetType from misfiring on those.
func subnetTypeStyleFor(value string, selected bool) (lipgloss.Style, bool) {
	switch strings.TrimSpace(value) {
	case "Public":
		if selected {
			return stateTextGoodSelected, true
		}
		return stateTextGood, true
	case "Private":
		if selected {
			return subnetTypeColorPrivateSelected, true
		}
		return subnetTypeColorPrivate, true
	default:
		return lipgloss.Style{}, false
	}
}

// colorizeSubnetType highlights the Subnet table's TYPE column — same
// post-render ansi.Cut splice as colorizeState/colorizeEdition (see
// colorizeState's doc for why this can't be done at Column.Get() time
// instead).
func colorizeSubnetType(view string, cols []table.Column) string {
	start, end, ok := columnRange(cols, "TYPE")
	if !ok {
		return view
	}

	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if i == 0 {
			continue // header row
		}
		selected := selectedLinePrefix != "" && strings.HasPrefix(line, selectedLinePrefix)
		mid := ansi.Strip(ansi.Cut(line, start, end))
		style, ok := subnetTypeStyleFor(mid, selected)
		if !ok {
			continue
		}
		left := ansi.Cut(line, 0, start)
		right := ansi.Cut(line, end, 1<<20)
		lines[i] = left + style.Render(mid) + right
	}
	return strings.Join(lines, "\n")
}
