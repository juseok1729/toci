package app

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/registry"
)

func TestVcnLabel(t *testing.T) {
	vcnID := "ocid1.vcn.oc1..aaa"
	names := map[string]vcnInfo{vcnID: {Name: "prod-vcn", Cidr: "10.0.0.0/16"}}

	cases := []struct {
		name string
		row  registry.Row
		want string
	}{
		{"known vcn", registry.Row{Raw: core.Subnet{VcnId: &vcnID}}, "prod-vcn"},
		{"unresolved vcn falls back to OCID", registry.Row{Raw: core.Subnet{VcnId: strPtr("ocid1.vcn.oc1..bbb")}}, "ocid1.vcn.oc1..bbb"},
		{"nil VcnId", registry.Row{Raw: core.Subnet{}}, ""},
		{"non-subnet raw", registry.Row{Raw: core.Vcn{}}, ""},
	}
	for _, c := range cases {
		if got := vcnLabel(c.row, names); got != c.want {
			t.Errorf("%s: vcnLabel() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestGroupRowsByVcn(t *testing.T) {
	vcnA, vcnB := "ocid1.vcn.oc1..a", "ocid1.vcn.oc1..b"
	names := map[string]vcnInfo{
		vcnA: {Name: "vcn-a", Cidr: "10.0.0.0/16"},
		vcnB: {Name: "vcn-b", Cidr: "10.1.0.0/16"},
	}
	rows := []registry.Row{
		{ID: "s3", Name: "s3", Raw: core.Subnet{VcnId: &vcnB}},
		{ID: "s1", Name: "s1", Raw: core.Subnet{VcnId: &vcnA}},
		{ID: "s2", Name: "s2", Raw: core.Subnet{VcnId: &vcnA}},
	}

	grouped := groupRowsByVcn(rows, names)

	wantOrder := []string{"vcn-header:vcn-a", "s1", "s2", "vcn-header:vcn-b", "s3"}
	if len(grouped) != len(wantOrder) {
		t.Fatalf("groupRowsByVcn() returned %d rows, want %d: %v", len(grouped), len(wantOrder), grouped)
	}
	for i, id := range wantOrder {
		if grouped[i].ID != id {
			t.Errorf("row %d: ID = %q, want %q", i, grouped[i].ID, id)
		}
	}
	headerA, ok := grouped[0].Raw.(vcnGroupHeader)
	if !ok {
		t.Fatalf("row 0 (%q) is not a vcnGroupHeader", grouped[0].ID)
	}
	if headerA.cidr != "10.0.0.0/16" {
		t.Errorf("vcn-a header cidr = %q, want %q", headerA.cidr, "10.0.0.0/16")
	}
	headerB, ok := grouped[3].Raw.(vcnGroupHeader)
	if !ok {
		t.Fatalf("row 3 (%q) is not a vcnGroupHeader", grouped[3].ID)
	}
	if headerB.cidr != "10.1.0.0/16" {
		t.Errorf("vcn-b header cidr = %q, want %q", headerB.cidr, "10.1.0.0/16")
	}

	glyphs := treeGlyphs(grouped)
	if glyphs["s1"] != treeChildMid {
		t.Errorf("s1 glyph = %q, want mid-child %q (s2 follows it in the same group)", glyphs["s1"], treeChildMid)
	}
	if glyphs["s2"] != treeChildLast {
		t.Errorf("s2 glyph = %q, want last-child %q (next row is vcn-b's header)", glyphs["s2"], treeChildLast)
	}
	if glyphs["s3"] != treeChildLast {
		t.Errorf("s3 glyph = %q, want last-child %q (last row overall)", glyphs["s3"], treeChildLast)
	}
	if _, ok := glyphs["vcn-header:vcn-a"]; ok {
		t.Errorf("header row should not get a tree glyph")
	}
}

// TestTreeColumnsShowsVcnCidrOnHeaderRow is the actual feature request: the
// VCN group header row should show that VCN's own CIDR and IP range in the
// Subnet table's CIDR/IP RANGE columns, not just its name.
func TestTreeColumnsShowsVcnCidrOnHeaderRow(t *testing.T) {
	cols := registry.NewSubnetResource(nil).Columns()
	decorated := treeColumns(cols, map[string]string{})

	header := registry.Row{Raw: vcnGroupHeader{name: "vcn-a", cidr: "10.0.0.0/16"}}
	get := func(title string) string {
		for _, c := range decorated {
			if c.Header == title {
				return c.Get(header)
			}
		}
		t.Fatalf("no %q column in Subnet's Columns()", title)
		return ""
	}

	if got, want := get("NAME"), treeGroupIcon+"vcn-a"; got != want {
		t.Errorf("NAME = %q, want %q", got, want)
	}
	if got, want := get("CIDR"), "10.0.0.0/16"; got != want {
		t.Errorf("CIDR = %q, want %q", got, want)
	}
	if got, want := get("IP RANGE"), registry.CidrRange("10.0.0.0/16"); got != want {
		t.Errorf("IP RANGE = %q, want %q", got, want)
	}
}
