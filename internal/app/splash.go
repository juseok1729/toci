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

// splashLogoStyle colors the splash screen's logo and filled progress bar
// only — kept separate from the shared titleStyle (used elsewhere for
// headers, table borders, etc.) so tuning the splash's look doesn't tint
// the rest of the UI. 196 is OCI's own brand red.
var splashLogoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

// splashMutedStyle/splashProfileStyle are the splash screen's own copies of
// the shared statusStyle/pathStyle (same values model.go's had for a long
// time) — split out so the splash screen's look stays fixed regardless of
// whatever palette experiments the main UI's shared styles go through.
var (
	splashMutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	splashProfileStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// spinnerFrames — same 4-frame Braille spinner taws
// (github.com/huseyinbabal/taws, src/ui/splash.rs) uses next to its status
// line. It advances once per stage change (see Model.splashSpinnerFrame),
// not every tick, matching taws's own SplashState::set_message, which bumps
// the frame each time the message changes rather than continuously.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸"}

var spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // taws renders its spinner in yellow

// splashPhrases are the witty status lines shown next to the spinner while
// loading. A new one is picked at random each time the fake bar advances to
// a new stage (see the splashTickMsg case in model.go) — not meant to
// convey real progress (there's only one real signal: the initial List call
// finishing), just to read as alive instead of a bare percentage.
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

// splashBarMargin matches taws's own bar sizing (src/ui/splash.rs,
// render_loading_bar: `area.width.saturating_sub(20)`) — the bar spans
// (terminal width - 20) instead of a fixed column count, so it keeps the
// same near-full-width proportion taws has regardless of terminal size.
const (
	splashBarMargin   = 20
	splashBarMinWidth = 10
)

// splashStages are the discrete levels the fake progress bar jumps through
// (10% -> 30% -> 60%) instead of creeping up one tick at a time. The bar
// holds at the second-to-last value (60%) until the real load finishes (see
// splashDataReady in model.go), then the tick after that snaps straight to
// 100% — no intermediate step for that last jump.
var splashStages = []int{10, 30, 60, 100}

// splashStageTicks[i] is how many 60ms ticks splashStages[i] is held before
// advancing to splashStages[i+1] — one entry per transition, so
// len(splashStageTicks) == len(splashStages)-1. Not uniform on purpose: the
// early jumps (10% -> 30% -> 60%) are quick, so the bar reads as leaping
// ahead rather than crawling evenly, and only the final hold (60%, waiting
// on splashDataReady) takes the real remaining time. Their sum (15 ticks *
// 60ms = 900ms) is the minimum time the splash holds before it's allowed to
// complete.
var splashStageTicks = []int{3, 3, 9}

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

	logo := splashLogoStyle.Render(asciiLogo)
	subtitle := splashMutedStyle.Render("Terminal UI for Oracle Cloud Infrastructure")
	profile := splashProfileStyle.Render(m.profile)

	barWidth := m.width - splashBarMargin
	if barWidth < splashBarMinWidth {
		barWidth = splashBarMinWidth
	}
	filled := m.splashProgress * barWidth / 100
	bar := "[" +
		splashLogoStyle.Render(strings.Repeat("█", filled)) +
		splashMutedStyle.Render(strings.Repeat("░", barWidth-filled)) +
		fmt.Sprintf("] %3d%%", m.splashProgress)

	icon, phrase := spinnerStyle.Render(spinnerFrames[m.splashSpinnerFrame%len(spinnerFrames)]), m.splashPhrase
	if m.splashDataReady {
		icon, phrase = "✅", "Ready!"
	}
	status := fmt.Sprintf("%s %s", icon, splashMutedStyle.Render(phrase))

	content := lipgloss.JoinVertical(lipgloss.Center,
		logo, "", subtitle, profile, "", bar, "", status,
	)

	// taws's layout (src/ui/splash.rs, render) sits its content well above
	// dead center rather than splitting leftover space evenly.
	return lipgloss.Place(m.width, m.height, lipgloss.Center, splashVerticalPosition, content)
}

// splashVerticalPosition is *high*, not low, to push the content block
// *up* — counter-intuitive, but lipgloss.Place's vertical Position is
// inverted from what Top/Center/Bottom's own values (0/0.5/1) suggest:
// PlaceVertical's non-exact-boundary case computes the top margin as
// gap*(1-pos), so increasing pos shrinks the top margin. Calibrated by
// rendering and counting blank lines, not derived analytically.
const splashVerticalPosition = lipgloss.Position(0.65)
