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

const splashBarWidth = 40

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
