package app

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/registry"
)

func TestVcnLabel(t *testing.T) {
	vcnID := "ocid1.vcn.oc1..aaa"
	names := map[string]string{vcnID: "prod-vcn"}

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
	names := map[string]string{vcnA: "vcn-a", vcnB: "vcn-b"}
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
	if _, ok := grouped[0].Raw.(vcnGroupHeader); !ok {
		t.Errorf("row 0 (%q) is not a vcnGroupHeader", grouped[0].ID)
	}
	if _, ok := grouped[3].Raw.(vcnGroupHeader); !ok {
		t.Errorf("row 3 (%q) is not a vcnGroupHeader", grouped[3].ID)
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
