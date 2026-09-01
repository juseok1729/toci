package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// asciiLogo is the "ANSI Shadow" figlet rendering of "TOCI".
const asciiLogo = `████████╗ ██████╗  ██████╗██╗
╚══██╔══╝██╔═══██╗██╔════╝██║
   ██║   ██║   ██║██║     ██║
   ██║   ██║   ██║██║     ██║
   ██║   ╚██████╔╝╚██████╗██║
   ╚═╝    ╚═════╝  ╚═════╝╚═╝`

// spinnerFrames cycles through the clock face emoji so the "hand" visibly
// spins — reads as a buffering/loading icon at a glance, unlike a plain
// Braille spinner.
var spinnerFrames = []string{"🕛", "🕐", "🕑", "🕒", "🕓", "🕔", "🕕", "🕖", "🕗", "🕘", "🕙", "🕚"}

// splashPhrases are the witty status lines shown next to the spinner while
// loading — picked once per run (see Model.splashPhrase in model.go), not
// meant to convey real progress (there's only one real signal: the initial
// List call finishing), just to read as alive instead of a bare percentage.
var splashPhrases = []string{
	"Waking up the tenancy...",
	"Politely interrogating OCI...",
	"Untangling VCNs...",
	"Summoning compartments...",
	"Reticulating route tables...",
	"Herding subnets...",
	"Consulting the region oracle...",
	"Counting cloud sheep...",
	"Buffering the buffer...",
	"Negotiating with IAM...",
	"Asking nicely for bytes...",
	"Spinning up the hamster wheel...",
}

const splashBarWidth = 90

// splashStages are the discrete levels the fake progress bar jumps through
// (10% -> 30% -> 60%) instead of creeping up one tick at a time. The bar
// holds at the second-to-last value (60%) until the real load finishes (see
// splashDataReady in model.go), then the tick after that snaps straight to
// 100% — no intermediate step for that last jump.
var splashStages = []int{10, 30, 60, 100}

// splashStageTicks[i] is how many 60ms ticks splashStages[i] is held before
// advancing to splashStages[i+1] — one entry per transition, so
// len(splashStageTicks) == len(splashStages)-1. Not uniform on purpose: the
// first jump (10% -> 30%) is noticeably quicker than the rest, so the bar
// reads as leaping ahead early rather than crawling evenly. Their sum (21
// ticks * 60ms = 1260ms) is the minimum time the splash holds before it's
// allowed to complete, same total as before this was made non-uniform.
var splashStageTicks = []int{3, 9, 9}

// splashStageIndex returns which splashStages entry should be showing at
// the given tick count, per splashStageTicks' cumulative thresholds.
func splashStageIndex(frame int) int {
	cum := 0
	for i, ticks := range splashStageTicks {
		cum += ticks
		if frame < cum {
			return i
		}
	}
	return len(splashStages) - 1
}

type splashTickMsg struct{}

func splashTickCmd() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg {
		return splashTickMsg{}
	})
}

func renderSplash(m Model) string {
	if m.width == 0 || m.height == 0 {
		return "toci"
	}

	logo := titleStyle.Render(asciiLogo)
	subtitle := statusStyle.Render("Terminal UI for Oracle Cloud Infrastructure")
	profile := pathStyle.Render(m.profile)

	filled := m.splashProgress * splashBarWidth / 100
	bar := "[" +
		titleStyle.Render(strings.Repeat("█", filled)) +
		statusStyle.Render(strings.Repeat("░", splashBarWidth-filled)) +
		fmt.Sprintf("] %3d%%", m.splashProgress)

	icon, phrase := spinnerFrames[m.splashFrame%len(spinnerFrames)], m.splashPhrase
	if m.splashDataReady {
		icon, phrase = "✅", "Ready!"
	}
	status := statusStyle.Render(fmt.Sprintf("%s %s", icon, phrase))

	content := lipgloss.JoinVertical(lipgloss.Center,
		logo, "", subtitle, profile, "", bar, "", status,
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
