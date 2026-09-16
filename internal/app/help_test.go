package app

import (
	"strings"
	"testing"

	"toci/internal/registry"
)

// TestHelpEntriesOmitsBasicMovement locks in the requested trim: j/k, the
// arrow keys, and shift+↑/↓ are all just list navigation/scrolling, not
// worth a which-key popup line.
func TestHelpEntriesOmitsBasicMovement(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}
	for _, e := range m.helpEntries() {
		if strings.Contains(e.desc, "move") || strings.Contains(e.key, "j/k") || strings.Contains(e.key, "shift+") {
			t.Errorf("expected basic movement/scrolling to be omitted from the help popup, found %+v", e)
		}
	}
}

// TestHelpEntriesListsSpaceSeparatelyFromColon: pressing space again while
// the popup is already showing (see TestDoubleSpaceOpensResourceSearch) is
// a second, independent way into the resource search — it gets its own
// popup line, labeled "⎵" (the popup is already up from the first space
// press, so this second one reads as just "space", not "space space"),
// rather than being merged into the ":" one, since a merged key column
// reads as one binding with two names, not two.
func TestHelpEntriesListsSpaceSeparatelyFromColon(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}

	var sawColon, sawSpace bool
	for _, e := range m.helpEntries() {
		if e.key == ":" {
			sawColon = true
		}
		if e.key == "⎵" {
			sawSpace = true
		}
	}
	if !sawColon {
		t.Error(`expected a ":" entry`)
	}
	if !sawSpace {
		t.Error(`expected a separate "⎵" entry`)
	}
}

// TestHelpEntriesEveryEntryHasAnIcon and TestRenderHelpBoxShowsArrowAndIcon
// check the LazyVim-style "key → icon description" layout requested for
// the which-key popup.
func TestHelpEntriesEveryEntryHasAnIcon(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}
	entries := m.helpEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least one help entry")
	}
	for _, e := range entries {
		if e.icon == "" {
			t.Errorf("entry %+v has no icon", e)
		}
	}
}

func TestRenderHelpBoxShowsArrowAndIcon(t *testing.T) {
	m := Model{resources: registry.All(nil), table: newTable(20)}
	out := renderHelpBox(m)

	if !strings.Contains(out, "→") {
		t.Error("expected the help box to show an arrow between each key and its description")
	}
	if !strings.Contains(out, "quit") {
		t.Error("expected the help box to still list \"quit\"")
	}
}
