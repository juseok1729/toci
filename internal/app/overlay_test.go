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
