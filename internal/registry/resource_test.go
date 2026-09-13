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

// TestSanitizeVolumeSize guards the fix for a reported issue: a File
// Storage (NFS) mount reports its size in the exabyte range rather than a
// real Boot/Block Volume's actual (<=32TB) size — if one ever turned up in
// a ListVolumes/ListBootVolumes response, summing it in would make an
// instance's total storage read as "8 exabytes" instead of its real disk
// size. Anything past maxSaneVolumeGB must be dropped, not counted.
func TestSanitizeVolumeSize(t *testing.T) {
	normal := int64(1024)                // a real 1TB block volume
	exabyteScale := int64(8_000_000_000) // ~8 exabytes in GB — a File Storage mount's reported size

	if got := sanitizeVolumeSize(&normal); got == nil || *got != normal {
		t.Errorf("sanitizeVolumeSize(%d) = %v, want %d unchanged", normal, got, normal)
	}
	if got := sanitizeVolumeSize(&exabyteScale); got != nil {
		t.Errorf("sanitizeVolumeSize(%d) = %v, want nil (excluded as implausible)", exabyteScale, *got)
	}
	if got := sanitizeVolumeSize(nil); got != nil {
		t.Errorf("sanitizeVolumeSize(nil) = %v, want nil", *got)
	}
}

func TestShortOS(t *testing.T) {
	cases := map[string]string{
		"Oracle Linux":            "OL",
		"Oracle Autonomous Linux": "OAL",
		"Canonical Ubuntu":        "Ubuntu",
		"Windows":                 "Win",
		"CentOS Linux":            "CentOS Linux", // no abbreviation — falls back to the raw value
		"":                        "",
	}
	for in, want := range cases {
		if got := shortOS(in); got != want {
			t.Errorf("shortOS(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortVersion(t *testing.T) {
	cases := map[string]string{
		"Server 2019 Standard":   "Svr 2019 Std",
		"Server 2022 Datacenter": "Svr 2022 DC",
		"8.9":                    "8.9", // Linux versions pass through unchanged
		"22.04":                  "22.04",
		"":                       "",
	}
	for in, want := range cases {
		if got := shortVersion(in); got != want {
			t.Errorf("shortVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestImageLabel guards a reported bug: OCI itself sets both
// OperatingSystem and OperatingSystemVersion to "Custom" for a custom
// image, so joining them unconditionally produced "Custom Custom" — the
// version is dropped whenever it duplicates the (abbreviated) OS.
func TestImageLabel(t *testing.T) {
	cases := []struct {
		os, version, want string
	}{
		{"Custom", "Custom", "Custom"},
		{"Oracle Linux", "8.9", "OL 8.9"},
		{"Windows", "Server 2019 Standard", "Win Svr 2019 Std"},
		{"Canonical Ubuntu", "", "Ubuntu"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := imageLabel(c.os, c.version); got != c.want {
			t.Errorf("imageLabel(%q, %q) = %q, want %q", c.os, c.version, got, c.want)
		}
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

func TestCidrRange(t *testing.T) {
	cases := map[string]string{
		"10.0.0.0/24":    "10.0.0.0 - 10.0.0.255 (254 usable)",
		"10.0.0.0/16":    "10.0.0.0 - 10.0.255.255 (65534 usable)",
		"10.0.0.0/31":    "10.0.0.0 - 10.0.0.1 (2 usable)",
		"192.168.1.5/32": "192.168.1.5 - 192.168.1.5 (1 usable)",
		"":               "",
		"not-a-cidr":     "",
	}
	for in, want := range cases {
		if got := cidrRange(in); got != want {
			t.Errorf("cidrRange(%q) = %q, want %q", in, got, want)
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
