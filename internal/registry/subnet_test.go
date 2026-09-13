package registry

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
)

func TestSubnetAccessLabel(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name string
		sn   core.Subnet
		want string
	}{
		{"prohibited -> private", core.Subnet{ProhibitPublicIpOnVnic: &yes}, "Private"},
		{"explicitly allowed -> public", core.Subnet{ProhibitPublicIpOnVnic: &no}, "Public"},
		{"unset (regional default) -> public", core.Subnet{}, "Public"},
	}
	for _, c := range cases {
		if got := subnetAccessLabel(c.sn); got != c.want {
			t.Errorf("%s: subnetAccessLabel() = %q, want %q", c.name, got, c.want)
		}
	}
}
