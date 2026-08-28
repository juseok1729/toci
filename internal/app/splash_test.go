package app

import (
	"strings"
	"testing"

	"toci/internal/registry"
)

func TestRenderSplash(t *testing.T) {
	m := Model{
		profile:        "WYD",
		mode:           modeSplash,
		width:          100,
		height:         30,
		splashProgress: 42,
		splashFrame:    3,
		resources:      []registry.Resource{registry.NewSubnetResource(nil)},
		scope:          registry.Scope{Region: "ap-chuncheon-1"},
	}

	out := renderSplash(m)
	for _, want := range []string{"████████╗", "42%", "WYD", "ap-chuncheon-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderSplash output missing %q", want)
		}
	}

	// Zero width/height happens before the first WindowSizeMsg arrives —
	// must not panic (lipgloss.Place with a 0 dimension is fine, but this
	// guards the explicit early-return path too).
	m.width, m.height = 0, 0
	if renderSplash(m) == "" {
		t.Error("renderSplash with zero dimensions returned empty string")
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
