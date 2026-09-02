package registry

import (
	"reflect"
	"testing"
)

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

func TestSumInstanceVolumeSizes(t *testing.T) {
	instanceToVolumes := map[string][]string{
		"inst1": {"boot1"},                       // boot volume only
		"inst2": {"boot2", "block2a", "block2b"}, // boot + two block volumes
		"inst3": {"vol-missing"},                 // size unresolved — dropped, not zero
	}
	volumeSize := map[string]int64{
		"boot1":   50,
		"boot2":   100,
		"block2a": 200,
		"block2b": 50,
	}

	got := sumInstanceVolumeSizes(instanceToVolumes, volumeSize)
	want := map[string]int64{"inst1": 50, "inst2": 350}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sumInstanceVolumeSizes() = %v, want %v", got, want)
	}
}
