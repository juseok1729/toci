package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"toci/internal/registry"
)

func TestRenderSplash(t *testing.T) {
	m := Model{
		profile:            "WYD",
		mode:               modeSplash,
		width:              100,
		height:             30,
		version:            "v1.2.3",
		splashFrame:        3,
		splashSpinnerFrame: 3,
		splashPhrase:       splashPhrases[0],
		resources:          []registry.Resource{registry.NewSubnetResource(nil)},
		scope:              registry.Scope{Region: "ap-chuncheon-1"},
	}

	out := renderSplash(m)
	for _, want := range []string{"████████╗", "WYD", splashPhrases[0], spinnerFrames[3%len(spinnerFrames)], "Find Resource", "f", "Quit", "q"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderSplash output missing %q", want)
		}
	}
	if strings.Contains(out, "⚡ TOCI v1.2.3") {
		t.Error("renderSplash should show the loading phrase, not the version, before splashDataReady")
	}

	m.splashDataReady = true
	if out := renderSplash(m); !strings.Contains(out, "⚡ TOCI v1.2.3") {
		t.Errorf("renderSplash with splashDataReady missing %q, got %q", "⚡ TOCI v1.2.3", out)
	}

	// Zero width/height happens before the first WindowSizeMsg arrives —
	// must not panic (lipgloss.Place with a 0 dimension is fine, but this
	// guards the explicit early-return path too).
	m.width, m.height = 0, 0
	if renderSplash(m) == "" {
		t.Error("renderSplash with zero dimensions returned empty string")
	}
}

func TestSplashVersionTextFallsBackToDev(t *testing.T) {
	if got := splashVersionText(Model{version: ""}); got != "⚡ TOCI dev" {
		t.Errorf("splashVersionText with empty version = %q, want %q", got, "⚡ TOCI dev")
	}
	if got := splashVersionText(Model{version: "dev"}); got != "⚡ TOCI dev" {
		t.Errorf("splashVersionText with dev version = %q, want %q", got, "⚡ TOCI dev")
	}
	if got := splashVersionText(Model{version: "v1.2.3"}); got != "⚡ TOCI v1.2.3" {
		t.Errorf("splashVersionText with a real version = %q, want %q", got, "⚡ TOCI v1.2.3")
	}
}

// TestSplashTickStopsOnceDataReady: the home screen is now persistent (no
// auto-transition away from it), so once fetchRootName finishes, ticking
// should just stop instead of driving the menu into another mode.
func TestSplashTickStopsOnceDataReady(t *testing.T) {
	m := Model{mode: modeSplash, resources: registry.All(nil)}

	for range 50 {
		mi, cmd := m.Update(splashTickMsg{})
		m = mi.(Model)
		if cmd == nil {
			t.Fatalf("splashTickMsg stopped ticking before splashDataReady")
		}
	}
	if m.mode != modeSplash {
		t.Fatalf("mode left modeSplash before splashDataReady")
	}

	m.splashDataReady = true
	mi, cmd := m.Update(splashTickMsg{})
	m = mi.(Model)
	if cmd != nil {
		t.Error("splashTickMsg kept ticking after splashDataReady, want it to stop")
	}
	if m.mode != modeSplash {
		t.Error("mode should stay modeSplash — the home screen no longer auto-navigates away")
	}
}

// TestSplashMenuKeysDispatch checks a couple of the home screen's menu keys
// actually do what their label says, rather than just rendering.
func TestSplashMenuKeysDispatch(t *testing.T) {
	m := Model{mode: modeSplash, resources: registry.All(nil)}

	// "f" opens the resource search, floating over the home menu itself
	// (pickerReturnMode = modeSplash) rather than the ordinary table/header
	// chrome the same picker floats over when opened from modeTable.
	mi, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m2 := mi.(Model)
	if m2.mode != modePicker || m2.picker.kind != pickerResource {
		t.Errorf("pressing %q on the home screen: mode = %v, picker.kind = %v, want modePicker/pickerResource", "f", m2.mode, m2.picker.kind)
	}
	if m2.pickerReturnMode != modeSplash {
		t.Errorf("pickerReturnMode = %v after pressing %q on the home screen, want modeSplash", m2.pickerReturnMode, "f")
	}
	// The search box's own height scales with the resource count (~26 rows
	// for all of them, categories included) and both it and the splash
	// logo center on similar height-proportional formulas, so at this
	// screen size the box can end up covering the logo itself — not
	// asserted on here since it's a coincidence of two independently-tuned
	// layouts, not a real contract. "Quit" (the menu's last row) reliably
	// sits below the box regardless, so it stands in for "this is the home
	// menu, not the ordinary table screen" instead.
	m2.width, m2.height = 100, 30
	out := ansi.Strip(m2.viewContent())
	if !strings.Contains(out, "Quit") {
		t.Errorf("viewContent for the home-screen search should show the home menu (e.g. \"Quit\") behind the search box, got:\n%s", out)
	}
	if strings.Contains(out, "Profile:") {
		// The ordinary header/table chrome — never shown for the
		// home-screen search, which has no resource loaded to describe.
		t.Errorf("viewContent for the home-screen search should not show the table header, got:\n%s", out)
	}

	// Esc closes it back onto the home menu, not the (still resourceless)
	// table underneath.
	mi, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m2b := mi.(Model)
	if m2b.mode != modeSplash {
		t.Errorf("mode after Esc from the home-screen search = %v, want modeSplash", m2b.mode)
	}

	// Picking a resource always lands on the table, regardless of where the
	// search was opened from.
	mi, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2c := mi.(Model)
	if m2c.mode != modeTable {
		t.Errorf("mode after Enter on the home-screen search = %v, want modeTable", m2c.mode)
	}

	// "i" jumps straight to Instances.
	mi, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m3 := mi.(Model)
	if m3.mode != modeTable || m3.current().Key() != "instance" {
		t.Errorf("pressing %q on the home screen: mode = %v, resource = %v, want modeTable/instance", "i", m3.mode, m3.current().Key())
	}
}

func TestAsciiLogoRowsSameWidth(t *testing.T) {
	rows := strings.Split(asciiLogo, "\n")
	width := len([]rune(rows[0]))
	for i, row := range rows {
		if got := len([]rune(row)); got != width {
			t.Errorf("asciiLogo row %d has width %d, want %d", i, got, width)
		}
	}
}
