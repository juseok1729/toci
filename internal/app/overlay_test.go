package app

import (
	"strings"
	"testing"
)

// A short base (e.g. the one-line "error: ..." state) used to make
// overlayCenter's row land past the end of baseLines, silently dropping the
// whole popup — see spliceOverlay's padding comment.
func TestOverlayCenterOnShortBase(t *testing.T) {
	base := "error: select a DRG first" // 1 line, nowhere near termHeight
	box := "+------+\n| pick |\n+------+"

	out := overlayCenter(base, box, 40, 24)

	if !strings.Contains(out, "pick") || !strings.Contains(out, "+------+") {
		t.Errorf("overlayCenter dropped the box against a short base:\n%s", out)
	}
}

// spliceOverlay never hands back a row wider than the terminal, even if a
// caller's own width bookkeeping (x, or the box's measured width) doesn't
// add up exactly — see the belt-and-suspenders clamp in spliceOverlay.
func TestSpliceOverlayClampsToTermWidth(t *testing.T) {
	baseLines := []string{strings.Repeat("x", 20)}
	boxLines := []string{"BOX"}

	// x=15 + boxWidth(3) = 18, well within termWidth(20) — but claim the
	// box is wider than it really is, forcing the "right" cut to start too
	// early and the reconstructed row to overshoot 20 columns.
	out := spliceOverlayForTest(baseLines, boxLines, 15, 0, 20, 30)

	if w := visibleWidth(out); w > 20 {
		t.Errorf("spliced row width = %d, want <= 20 (termWidth): %q", w, out)
	}
}

// spliceOverlayForTest lets the test simulate a boxLine whose true width
// doesn't match what spliceOverlay's own ansi.StringWidth would measure —
// exactly the kind of mismatch the clamp guards against — without needing
// an actual malformed ANSI string. Real callers always pass a boxLine
// spliceOverlay measures accurately; this just forces the drift.
func spliceOverlayForTest(baseLines, boxLines []string, x, y, termWidth, fakeBoxWidth int) string {
	// A boxLine that's genuinely fakeBoxWidth columns wide, so
	// ansi.StringWidth(boxLine) really does return more than the caller
	// intended to reserve for it.
	wide := make([]string, len(boxLines))
	for i, b := range boxLines {
		wide[i] = b + strings.Repeat(" ", fakeBoxWidth-len(b))
	}
	return spliceOverlay(append([]string{}, baseLines...), wide, x, y, termWidth)
}

func visibleWidth(s string) int {
	w := 0
	for _, line := range strings.Split(s, "\n") {
		if len([]rune(line)) > w {
			w = len([]rune(line))
		}
	}
	return w
}
