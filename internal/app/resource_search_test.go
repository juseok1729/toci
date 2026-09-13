package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"toci/internal/registry"
)

func TestRenderResourceSearch(t *testing.T) {
	m := Model{
		resources: registry.All(nil),
		width:     120,
		height:    40,
	}
	m.openResourceSearch()

	for _, width := range []int{120, 60, 50, 20, 0} {
		m.width = width
		out := ansi.Strip(m.renderResourceSearch())
		lines := strings.Split(out, "\n")
		for i, l := range lines {
			if i > 0 && len([]rune(l)) != len([]rune(lines[0])) {
				t.Errorf("width=%d: line %d has length %d, want %d (box must stay rectangular)\nfull box:\n%s", width, i, len([]rune(l)), len([]rune(lines[0])), out)
			}
		}
	}

	m.width = 120
	out := ansi.Strip(m.renderResourceSearch())
	if !strings.Contains(out, "Resources") {
		t.Error("renderResourceSearch missing title")
	}
	wantCount := fmt.Sprintf("%d/%d", len(m.resources), len(m.resources))
	if !strings.Contains(out, wantCount) {
		t.Errorf("renderResourceSearch missing match count %q\nfull box:\n%s", wantCount, out)
	}
	for _, category := range []string{"Compute", "Network", "Database", "Governance"} {
		if !strings.Contains(out, category) {
			t.Errorf("renderResourceSearch missing category header %q\nfull box:\n%s", category, out)
		}
	}
}
