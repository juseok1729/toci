package app

import (
	"reflect"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
)

func TestRouteTargetKind(t *testing.T) {
	cases := map[string]string{
		"ocid1.internetgateway.oc1.ap-seoul-1.aaaa": "Internet Gateway",
		"ocid1.natgateway.oc1.ap-seoul-1.aaaa":      "NAT Gateway",
		"ocid1.servicegateway.oc1.ap-seoul-1.aaaa":  "Service Gateway",
		"ocid1.drg.oc1.ap-seoul-1.aaaa":             "DRG",
		"ocid1.localpeeringgateway.oc1..aaaa":       "Local Peering GW",
		"ocid1.privateip.oc1.ap-seoul-1.aaaa":       "Private IP",
		"ocid1.somethingnew.oc1..aaaa":              "somethingnew", // unmapped kind falls back to the raw segment
		"":                                          "-",
		"not-an-ocid":                               "-",
	}
	for in, want := range cases {
		if got := routeTargetKind(in); got != want {
			t.Errorf("routeTargetKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRouteRuleRecords(t *testing.T) {
	rt := core.RouteTable{
		RouteRules: []core.RouteRule{
			{
				Destination:     strPtr("0.0.0.0/0"),
				NetworkEntityId: strPtr("ocid1.internetgateway.oc1.ap-seoul-1.aaaa"),
				RouteType:       core.RouteRuleRouteTypeStatic,
				Description:     strPtr("default route"),
			},
			{
				Destination:     strPtr("10.0.0.0/16"),
				NetworkEntityId: strPtr("ocid1.drg.oc1.ap-seoul-1.bbbb"),
				// RouteType left unset — should default to STATIC.
			},
		},
	}

	got := routeRuleRecords(rt)
	want := [][]string{
		{"0.0.0.0/0", "Internet Gateway", "STATIC", "default route"},
		{"10.0.0.0/16", "DRG", "STATIC", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("routeRuleRecords() = %v, want %v", got, want)
	}
}
