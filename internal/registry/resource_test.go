package registry

import (
	"reflect"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/database"
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

func TestDbSystemRoleLabel(t *testing.T) {
	cases := []struct {
		name       string
		nodeStates []string
		role       string
		want       string
	}{
		{"single node, no DG role", []string{"Available"}, "", "-"},
		{"single node, primary", []string{"Available"}, "Primary", "Primary"},
		{"single node, standby", []string{"Available"}, "Standby", "Standby"},
		{"no node info, no DG role", nil, "", "-"},
		{"2-node RAC wins over any role", []string{"Available", "Available"}, "Primary", "RAC"},
		{"2-node RAC, no role", []string{"Stopped", "Stopped"}, "", "RAC"},
	}
	for _, c := range cases {
		if got := dbSystemRoleLabel(c.nodeStates, c.role); got != c.want {
			t.Errorf("%s: dbSystemRoleLabel(%v, %q) = %q, want %q", c.name, c.nodeStates, c.role, got, c.want)
		}
	}
}

func TestRegionFromOCID(t *testing.T) {
	cases := map[string]string{
		"ocid1.dbsystem.oc1.ap-seoul-1.anuwgljrzwnc6yaad6d7gq7xpuapbijr45f6tnuqnmy2l4kns3pezwin45ea": "ap-seoul-1",
		"ocid1.tenancy.oc1..aaaaaaaas7p34lrvrfzqp7h2jc4z2uemrcl6howfntsntlb2ei6zvuivqfua":            "", // no region segment
		"":            "",
		"not-an-ocid": "",
	}
	for in, want := range cases {
		if got := regionFromOCID(in); got != want {
			t.Errorf("regionFromOCID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDbEditionAbbrev(t *testing.T) {
	cases := map[database.DbSystemSummaryDatabaseEditionEnum]string{
		database.DbSystemSummaryDatabaseEditionStandardEdition:                     "SE2",
		database.DbSystemSummaryDatabaseEditionEnterpriseEdition:                   "EE",
		database.DbSystemSummaryDatabaseEditionEnterpriseEditionHighPerformance:    "EE-HP",
		database.DbSystemSummaryDatabaseEditionEnterpriseEditionExtremePerformance: "EE-EP",
		database.DbSystemSummaryDatabaseEditionEnterpriseEditionDeveloper:          "EE-DEV",
		"SOME_FUTURE_EDITION": "SOME_FUTURE_EDITION", // unknown value falls back to the raw enum
	}
	for in, want := range cases {
		if got := dbEditionAbbrev(in); got != want {
			t.Errorf("dbEditionAbbrev(%q) = %q, want %q", in, got, want)
		}
	}
}
