package app

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestEditionStyleForExactMatchNotSubstring(t *testing.T) {
	// "EE" is a literal prefix of "EE-HP"/"EE-EP"/"EE-DEV" — this must be
	// exact-match, not Contains, or those would misfire as plain "EE".
	cases := map[string]lipgloss.Style{
		"SE2":    editionColorSE2,
		"EE":     editionColorEE,
		"EE-HP":  editionColorEEHP,
		"EE-EP":  editionColorEEEP,
		"EE-DEV": editionColorEEDev,
	}
	for value, want := range cases {
		got, ok := editionStyleFor(value, false)
		if !ok {
			t.Errorf("editionStyleFor(%q) matched no tier", value)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("editionStyleFor(%q) picked the wrong style", value)
		}
	}

	if _, ok := editionStyleFor("UNKNOWN_FUTURE_EDITION", false); ok {
		t.Error("editionStyleFor matched an unrecognized value, want no match")
	}
}
