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

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const splashBarWidth = 70

// splashStages are the discrete levels the fake progress bar jumps through
// (10% -> 30% -> 60%) instead of creeping up one tick at a time — every
// splashTicksPerStage ticks it advances to the next value. The bar holds at
// the second-to-last value (60%) until the real load finishes (see
// splashDataReady in model.go), then the tick after that snaps straight to
// 100% — no intermediate step for that last jump.
var splashStages = []int{10, 30, 60, 100}

// splashTicksPerStage * (len(splashStages)-1) * the 60ms tick interval is
// the minimum time the splash holds before it's allowed to complete
// (~1.2s): 7 * 3 * 60ms = 1260ms.
const splashTicksPerStage = 7

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

	spinner := spinnerFrames[m.splashFrame%len(spinnerFrames)]
	status := statusStyle.Render(fmt.Sprintf("%s Fetching %s from %s", spinner, strings.ToLower(m.current().Label()), m.scope.Region))

	content := lipgloss.JoinVertical(lipgloss.Center,
		logo, "", subtitle, profile, "", bar, "", status,
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
