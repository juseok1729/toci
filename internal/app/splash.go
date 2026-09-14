package app

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// asciiLogo is the "ANSI Shadow" figlet rendering of "TOCI".
const asciiLogo = `████████╗ ██████╗  ██████╗██╗
╚══██╔══╝██╔═══██╗██╔════╝██║
   ██║   ██║   ██║██║     ██║
   ██║   ██║   ██║██║     ██║
   ██║   ╚██████╔╝╚██████╗██║
   ╚═╝    ╚═════╝  ╚═════╝╚═╝`

// splashLogoStyle colors the splash screen's logo (and, via cornerLogo in
// model.go, the small corner wordmark on every other screen) — kept
// separate from the shared titleStyle (used elsewhere for headers, table
// borders, etc.) so tuning the splash's look doesn't tint the rest of the
// UI. #ff4d4d sits about a third of the way from OCI's own brand red (pure
// red, #ff0000) toward white — a true-color hex instead of a 256-palette
// index so it can land at a precise point between them.
var splashLogoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#fc6464")).Bold(true)

// splashMenuLabelStyle colors the home screen's menu labels — a bright red,
// one small step lighter than plain 203, so the menu reads as part of the
// same red brand as the logo without going as pale as a full palette step
// up (210). A true-color hex, not a palette index, so it can land between
// them.
var splashMenuLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#fc6464"))

// splashMenuKeyStyle colors the home screen's menu keys ("f", "i", ...) — a
// pale yellow (228), lighter than spinnerStyle's saturated gold (220) below,
// which read as too dark/heavy for a key that's on screen constantly rather
// than briefly like the loading spinner.
var splashMenuKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("228")).Bold(true)

// splashProfileStyle is the splash screen's own copy of the shared
// pathStyle (same value model.go's had for a long time) — split out so the
// splash screen's look stays fixed regardless of whatever palette
// experiments the main UI's shared styles go through.
var (
	splashProfileStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	// splashPhraseStyle is the bright white for the status line under the
	// menu (spinner phrase while loading), so it pops against the dimmer
	// subtitle/profile above it.
	splashPhraseStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
)

// spinnerFrames — same 4-frame Braille spinner taws
// (github.com/huseyinbabal/taws, src/ui/splash.rs) uses next to its status
// line. It advances once per phrase change (see splashPhraseEveryTicks), not
// every tick, matching taws's own SplashState::set_message, which bumps the
// frame each time the message changes rather than continuously.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸"}

var spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // taws renders its spinner in yellow

// splashPhrases are the witty status lines shown at the bottom of the home
// screen while fetchRootName is still in flight. A new one is picked every
// splashPhraseEveryTicks (see splashTickMsg in model.go) — not meant to
// convey real progress (there's only one real signal: fetchRootName
// finishing), just to read as alive instead of a static "Loading...".
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

// splashPhraseEveryTicks paces the phrase/spinner rotation — 15 ticks *
// 60ms = 900ms per phrase, matching the old splash's average stage hold.
const splashPhraseEveryTicks = 15

// splashMenuItem is one row of the home screen's LazyVim-style menu: an
// icon, a label, the key that triggers it, and what pressing that key does.
// icon is a Nerd Font glyph (Private Use Area codepoint, Font Awesome
// subset) — like LazyVim's own dashboard, it only renders as an icon in a
// terminal using a Nerd Font; anywhere else it shows as a blank/placeholder
// box.
type splashMenuItem struct {
	icon   string
	label  string
	key    string
	action func(m *Model) tea.Cmd
}

// splashResourceAction jumps straight to a resource kind (by registry.Key())
// without going through the "f" search — the home screen's fast path for
// the handful of resources common enough to deserve their own key.
func splashResourceAction(key string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		for i, r := range m.resources {
			if r.Key() == key {
				m.mode = modeTable
				return m.switchResource(i)
			}
		}
		return nil
	}
}

// splashMenuItems is the home screen's menu, top to bottom. "Find Resource"
// (the fuzzy "f" search) covers every resource kind; the rest are one-key
// shortcuts to the ones used often enough to skip the search for. Add more
// here as needed — same shape as resourceCategories in resource_picker.go.
var splashMenuItems = []splashMenuItem{
	{"", "Find Resource", "f", func(m *Model) tea.Cmd {
		m.openResourceSearch()
		// Esc (or picking nothing) should close back onto the home menu,
		// not drop into the table underneath — there's no resource loaded
		// there yet, see openResourceSearch's default.
		m.pickerReturnMode = modeSplash
		return nil
	}}, // nf-fa-search
	{"", "Instances", "i", splashResourceAction("instance")},           // nf-fa-server
	{"", "VCNs", "v", splashResourceAction("vcn")},                     // nf-fa-sitemap
	{"", "OKE Clusters", "k", splashResourceAction("oke")},             // nf-fa-cubes
	{"", "DB Systems", "d", splashResourceAction("db-system")},         // nf-fa-database
	{"", "Exascale", "e", splashResourceAction("exascale")},            // nf-fa-database
	{"", "Security Lists", "s", splashResourceAction("security-list")}, // nf-fa-shield
	{"", "Quit", "q", func(m *Model) tea.Cmd { return tea.Quit }},      // nf-fa-sign-out
}

type splashTickMsg struct{}

func splashTickCmd() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg {
		return splashTickMsg{}
	})
}

// splashMenuWidth is the fixed width of each menu row (icon  label ... key)
// — wide, like LazyVim's own dashboard, so the row spans well past the logo
// instead of collapsing into a small block that reads as huddled in the
// middle of an otherwise-empty screen.
const splashMenuWidth = 60

// splashMenuView renders the label/key rows: labels in splashMenuLabelStyle
// (bright red) padded out to splashMenuWidth, keys right-aligned in
// splashMenuKeyStyle (yellow).
func splashMenuView() string {
	lines := make([]string, len(splashMenuItems))
	for i, it := range splashMenuItems {
		// Two spaces, not one — some Nerd Font glyphs (e.g. the server/
		// database/shield icons) are drawn flush to the right edge of
		// their cell, so a single space reads as glued to the label.
		iconAndLabel := it.icon + "  " + it.label
		label := splashMenuLabelStyle.Render(iconAndLabel)
		key := splashMenuKeyStyle.Render(it.key)
		gap := splashMenuWidth - lipgloss.Width(iconAndLabel) - lipgloss.Width(it.key)
		if gap < 2 {
			gap = 2
		}
		lines[i] = label + strings.Repeat(" ", gap) + key
	}
	// A blank line between rows, not just between the icon/label and key —
	// packed tight to the line above/below, it read as a wall of text
	// instead of a menu.
	return strings.Join(lines, "\n\n")
}

// splashVersionText is what the bottom status line settles on once
// fetchRootName finishes — "TOCI dev" for a local/unversioned build, "TOCI
// vX.Y.Z" for a real release (see cmd/toci/main.go's version var).
func splashVersionText(m Model) string {
	v := m.version
	if v == "" {
		v = "dev"
	}
	return "⚡ TOCI " + v
}

func renderSplash(m Model) string {
	if m.width == 0 || m.height == 0 {
		return "toci"
	}

	logo := splashLogoStyle.Render(asciiLogo)
	subtitle := splashLogoStyle.Render("Terminal UI for Oracle Cloud Infrastructure")
	profile := splashProfileStyle.Render(" " + m.profile) // nf-fa-user
	menu := splashMenuView()

	status := splashPhraseStyle.Render(splashVersionText(m))
	if !m.splashDataReady {
		icon := spinnerStyle.Render(spinnerFrames[m.splashSpinnerFrame%len(spinnerFrames)])
		status = fmt.Sprintf("%s %s", icon, splashPhraseStyle.Render(m.splashPhrase))
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		// Leading "", "" shifts the whole block (logo through menu) down by
		// two rows; the extra "" before menu (one more than subtitle/profile
		// get) nudges just the menu a bit further below the profile line.
		"", "", logo, "", subtitle, profile, "", "", "", menu, "", status,
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
