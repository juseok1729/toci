package app

import (
	"strings"
	"testing"

	"toci/internal/registry"
)

func TestRenderSplash(t *testing.T) {
	m := Model{
		profile:            "WYD",
		mode:               modeSplash,
		width:              100,
		height:             30,
		splashProgress:     42,
		splashFrame:        3,
		splashSpinnerFrame: 3,
		splashPhrase:       splashPhrases[0],
		resources:          []registry.Resource{registry.NewSubnetResource(nil)},
		scope:              registry.Scope{Region: "ap-chuncheon-1"},
	}

	out := renderSplash(m)
	for _, want := range []string{"████████╗", "42%", "WYD", splashPhrases[0], spinnerFrames[3%len(spinnerFrames)]} {
		if !strings.Contains(out, want) {
			t.Errorf("renderSplash output missing %q", want)
		}
	}

	m.splashDataReady = true
	if out := renderSplash(m); !strings.Contains(out, "Ready!") {
		t.Errorf("renderSplash with splashDataReady missing %q, got %q", "Ready!", out)
	}

	// Zero width/height happens before the first WindowSizeMsg arrives —
	// must not panic (lipgloss.Place with a 0 dimension is fine, but this
	// guards the explicit early-return path too).
	m.width, m.height = 0, 0
	if renderSplash(m) == "" {
		t.Error("renderSplash with zero dimensions returned empty string")
	}
}

func TestSplashProgressStagesAndHold(t *testing.T) {
	m := Model{mode: modeSplash}
	holdCap := splashStages[len(splashStages)-2]

	seen := map[int]bool{}
	for range 200 {
		mi, _ := m.Update(splashTickMsg{})
		m = mi.(Model)
		seen[m.splashProgress] = true
		if m.splashProgress > holdCap {
			t.Fatalf("splashProgress reached %d before splashDataReady, want capped at %d", m.splashProgress, holdCap)
		}
		if m.mode != modeSplash {
			t.Fatalf("mode left modeSplash before splashDataReady")
		}
	}
	for _, want := range splashStages[:len(splashStages)-1] {
		if !seen[want] {
			t.Errorf("splashProgress never hit stage value %d, got values %v", want, seen)
		}
	}
	if seen[100] {
		t.Error("splashProgress hit 100 before splashDataReady was set")
	}

	m.splashDataReady = true
	for range 100 {
		if m.mode != modeSplash {
			break
		}
		mi, _ := m.Update(splashTickMsg{})
		m = mi.(Model)
	}
	if m.mode != modeTable {
		t.Errorf("mode = %v after splashDataReady, want modeTable", m.mode)
	}
	if m.splashProgress != 100 {
		t.Errorf("splashProgress = %d when leaving splash, want 100", m.splashProgress)
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
