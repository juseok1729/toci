package registry

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
)

func TestDrgAttachmentType(t *testing.T) {
	cases := []struct {
		name string
		nd   core.DrgAttachmentNetworkDetails
		want string
	}{
		{"vcn", core.VcnDrgAttachmentNetworkDetails{}, "VCN"},
		{"ipsec tunnel", core.IpsecTunnelDrgAttachmentNetworkDetails{}, "IPSec Tunnel"},
		{"virtual circuit", core.VirtualCircuitDrgAttachmentNetworkDetails{}, "Virtual Circuit"},
		{"remote peering", core.RemotePeeringConnectionDrgAttachmentNetworkDetails{}, "Remote Peering"},
		{"loopback", core.LoopBackDrgAttachmentNetworkDetails{}, "Loopback"},
		{"nil (older attachment predating NetworkDetails)", nil, "-"},
	}
	for _, c := range cases {
		if got := drgAttachmentType(c.nd); got != c.want {
			t.Errorf("%s: drgAttachmentType() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDrgAttachmentTargetID(t *testing.T) {
	id := "ocid1.vcn.oc1..aaa"
	legacyID := "ocid1.vcn.oc1..legacy"

	cases := []struct {
		name string
		a    core.DrgAttachment
		want string
	}{
		{"NetworkDetails present", core.DrgAttachment{NetworkDetails: core.VcnDrgAttachmentNetworkDetails{Id: &id}}, id},
		{"nil NetworkDetails falls back to deprecated VcnId", core.DrgAttachment{VcnId: &legacyID}, legacyID},
		{"neither set", core.DrgAttachment{}, ""},
	}
	for _, c := range cases {
		if got := drgAttachmentTargetID(c.a); got != c.want {
			t.Errorf("%s: drgAttachmentTargetID() = %q, want %q", c.name, got, c.want)
		}
	}
}
