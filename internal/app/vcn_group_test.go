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
