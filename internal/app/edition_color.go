package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Edition tier colors — not a health signal like Good/Bad/Warn, so this is
// deliberately a separate palette from state_color.go's. Roughly a ladder,
// all picked at high saturation ("강렬한 색") rather than muted tones:
// EE-DEV (free, no license cost) is vivid green; the paid ladder SE2 → EE
// → EE-HP → EE-EP goes cyan → blue → purple → hot pink, the last two per
// explicit request.
var (
	editionColorSE2   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00E5FF"))
	editionColorEE    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2979FF"))
	editionColorEEHP  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AA00FF"))
	editionColorEEEP  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF1493"))
	editionColorEEDev = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00E676"))

	// Selected variants additionally set Background(ociSelBg) — same reason
	// as state_color.go's stateText*Selected styles: without it,
	// style.Render's own trailing reset would punch a default-background
	// hole in the selected row right where EDITION sits.
	//
	// A solid black backdrop, then same-hue-but-darker, were both tried
	// here first — see git history — before landing on pastel/light tints
	// of each tier's hue: high lightness reads clearly against ociSelBg's
	// medium-tone green by contrast, without the vivid non-selected colors
	// competing with it in saturation. EE-DEV's tint happens to equal
	// ociHighlt (the app's gold accent), so it reuses that constant instead
	// of duplicating the hex.
	editionColorSE2Selected   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7DF9FF")).Background(lipgloss.Color(ociSelBg))
	editionColorEESelected    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AFD7FF")).Background(lipgloss.Color(ociSelBg))
	editionColorEEHPSelected  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2C4FF")).Background(lipgloss.Color(ociSelBg))
	editionColorEEEPSelected  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFB8DE")).Background(lipgloss.Color(ociSelBg))
	editionColorEEDevSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ociHighlt)).Background(lipgloss.Color(ociSelBg))
)

// editionStyleFor maps a DB System EDITION cell's already-abbreviated text
// (see registry.dbEditionAbbrev: "SE2"/"EE"/"EE-HP"/"EE-EP"/"EE-DEV") to its
// color. Matched by exact equality, not substring — "EE" is a literal
// prefix of "EE-HP"/"EE-EP"/"EE-DEV", so Contains would misfire the way
// state_color.go's Warn-before-Good ordering has to guard against.
func editionStyleFor(value string, selected bool) (lipgloss.Style, bool) {
	switch strings.TrimSpace(value) {
	case "SE2":
		if selected {
			return editionColorSE2Selected, true
		}
		return editionColorSE2, true
	case "EE":
		if selected {
			return editionColorEESelected, true
		}
		return editionColorEE, true
	case "EE-HP":
		if selected {
			return editionColorEEHPSelected, true
		}
		return editionColorEEHP, true
	case "EE-EP":
		if selected {
			return editionColorEEEPSelected, true
		}
		return editionColorEEEP, true
	case "EE-DEV":
		if selected {
			return editionColorEEDevSelected, true
		}
		return editionColorEEDev, true
	default:
		return lipgloss.Style{}, false
	}
}

// colorizeEdition highlights the DB System table's EDITION column — same
// post-render ansi.Cut splice as colorizeState (see its doc for why this
// can't be done at Column.Get() time instead).
func colorizeEdition(view string, cols []table.Column) string {
	start, end, ok := columnRange(cols, "EDITION")
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
		style, ok := editionStyleFor(mid, selected)
		if !ok {
			continue
		}
		left := ansi.Cut(line, 0, start)
		right := ansi.Cut(line, end, 1<<20)
		lines[i] = left + style.Render(mid) + right
	}
	return strings.Join(lines, "\n")
}
