package registry

import "testing"

func TestStateLabel(t *testing.T) {
	cases := map[string]string{
		"RUNNING":         "Running",
		"STOPPED":         "Stopped",
		"NEEDS_ATTENTION": "Needs Attention",
		"":                "",
	}
	for in, want := range cases {
		if got := stateLabel(in); got != want {
			t.Errorf("stateLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
