package registry

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
)

func TestRouteTableAccessLabel(t *testing.T) {
	igw := "ocid1.internetgateway.oc1.ap-seoul-1.aaaa"
	nat := "ocid1.natgateway.oc1.ap-seoul-1.bbbb"
	drg := "ocid1.drg.oc1.ap-seoul-1.cccc"

	cases := []struct {
		name string
		rt   core.RouteTable
		want string
	}{
		{"has an internet gateway route -> public", core.RouteTable{RouteRules: []core.RouteRule{
			{NetworkEntityId: &igw},
		}}, "Public"},
		{"internet gateway alongside other routes -> public", core.RouteTable{RouteRules: []core.RouteRule{
			{NetworkEntityId: &nat},
			{NetworkEntityId: &igw},
		}}, "Public"},
		{"only NAT/DRG routes -> private", core.RouteTable{RouteRules: []core.RouteRule{
			{NetworkEntityId: &nat},
			{NetworkEntityId: &drg},
		}}, "Private"},
		{"no rules -> private", core.RouteTable{}, "Private"},
	}
	for _, c := range cases {
		if got := routeTableAccessLabel(c.rt); got != c.want {
			t.Errorf("%s: routeTableAccessLabel() = %q, want %q", c.name, got, c.want)
		}
	}
}
