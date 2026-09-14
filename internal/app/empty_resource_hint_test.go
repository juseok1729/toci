package app

import (
	"errors"
	"strings"
	"testing"

	"toci/internal/registry"
)

func TestShowsEmptyResourceHint(t *testing.T) {
	base := Model{mode: modeTable, resources: []registry.Resource{registry.NewInstanceResource(nil)}}

	loading := base
	loading.loading = true
	if loading.showsEmptyResourceHint() {
		t.Error("showsEmptyResourceHint should be false while still loading")
	}

	errored := base
	errored.err = errors.New("boom")
	if errored.showsEmptyResourceHint() {
		t.Error("showsEmptyResourceHint should be false when the fetch errored")
	}

	filtered := base
	filtered.filterQuery = "x"
	if filtered.showsEmptyResourceHint() {
		t.Error("showsEmptyResourceHint should be false when a filter query, not an empty compartment, explains the empty table")
	}

	nonEmpty := base
	nonEmpty.displayRows = []registry.Row{{ID: "1"}}
	if nonEmpty.showsEmptyResourceHint() {
		t.Error("showsEmptyResourceHint should be false when there are rows")
	}

	if !base.showsEmptyResourceHint() {
		t.Error("showsEmptyResourceHint should be true: modeTable, not loading, no error, no filter, zero rows")
	}
}

func TestRenderEmptyResourceHintMentionsSubtreeOnlyWhenOff(t *testing.T) {
	m := Model{resources: []registry.Resource{registry.NewInstanceResource(nil)}}

	out := m.renderEmptyResourceHint()
	if !strings.Contains(out, "shift+c") {
		t.Errorf("hint with subtree off should mention shift+c, got:\n%s", out)
	}

	m.subtreeOn = true
	out = m.renderEmptyResourceHint()
	if strings.Contains(out, "shift+c") {
		t.Errorf("hint with subtree already on should not suggest enabling it again, got:\n%s", out)
	}
	if !strings.Contains(out, "different compartment") {
		t.Errorf("hint with subtree on should still suggest picking a different compartment, got:\n%s", out)
	}
}
