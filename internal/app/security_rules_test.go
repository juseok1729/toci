package app

import (
	"reflect"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// TestSecurityRuleRecords covers the 8-column layout (DIR, STATELESS,
// SOURCE, PROTOCOL, SOURCE PORT, DEST PORT, TYPE AND CODE, DESCRIPTION):
// an ingress TCP rule with only a destination port restricted, an egress
// "all protocols" rule with no port/type-code columns at all, and an
// ICMP rule using TYPE AND CODE instead of ports.
func TestSecurityRuleRecords(t *testing.T) {
	sl := core.SecurityList{
		IngressSecurityRules: []core.IngressSecurityRule{
			{
				Protocol: strPtr("6"),
				Source:   strPtr("10.0.0.0/24"),
				TcpOptions: &core.TcpOptions{
					DestinationPortRange: &core.PortRange{Min: intPtr(22), Max: intPtr(22)},
				},
				Description: strPtr("ssh in"),
			},
			{
				Protocol: strPtr("1"), // ICMP
				Source:   strPtr("0.0.0.0/0"),
				IcmpOptions: &core.IcmpOptions{
					Type: intPtr(3),
					Code: intPtr(4),
				},
			},
		},
		EgressSecurityRules: []core.EgressSecurityRule{
			{
				Protocol:    strPtr("all"),
				Destination: strPtr("0.0.0.0/0"),
				IsStateless: boolPtr(true),
			},
		},
	}

	got := securityRuleRecords(sl)
	want := [][]string{
		{"INGRESS", "no", "10.0.0.0/24", "TCP", "All", "22", "", "ssh in"},
		{"INGRESS", "no", "0.0.0.0/0", "ICMP", "", "", "3:4", ""},
		{"EGRESS", "yes", "0.0.0.0/0", "ALL", "", "", "", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("securityRuleRecords() = %v, want %v", got, want)
	}
}

func TestIcmpTypeCodeString(t *testing.T) {
	cases := []struct {
		name     string
		protocol string
		opts     *core.IcmpOptions
		want     string
	}{
		{"non-ICMP protocol", "6", nil, ""},
		{"ICMP, no options (all types/codes allowed)", "1", nil, "All"},
		{"ICMPv6, no options", "58", nil, "All"},
		{"ICMP, type only", "1", &core.IcmpOptions{Type: intPtr(8)}, "8"},
		{"ICMP, type and code", "1", &core.IcmpOptions{Type: intPtr(3), Code: intPtr(4)}, "3:4"},
	}
	for _, c := range cases {
		if got := icmpTypeCodeString(c.protocol, c.opts); got != c.want {
			t.Errorf("%s: icmpTypeCodeString(%q, %+v) = %q, want %q", c.name, c.protocol, c.opts, got, c.want)
		}
	}
}

func TestDirectionalPortString(t *testing.T) {
	tcpBothRestricted := &core.TcpOptions{
		SourcePortRange:      &core.PortRange{Min: intPtr(1024), Max: intPtr(65535)},
		DestinationPortRange: &core.PortRange{Min: intPtr(443), Max: intPtr(443)},
	}
	if got := srcPortString(tcpBothRestricted, nil); got != "1024-65535" {
		t.Errorf("srcPortString() = %q, want %q", got, "1024-65535")
	}
	if got := dstPortString(tcpBothRestricted, nil); got != "443" {
		t.Errorf("dstPortString() = %q, want %q", got, "443")
	}

	tcpUnrestricted := &core.TcpOptions{}
	if got := srcPortString(tcpUnrestricted, nil); got != "All" {
		t.Errorf("srcPortString() with no SourcePortRange = %q, want %q", got, "All")
	}

	if got := srcPortString(nil, nil); got != "" {
		t.Errorf("srcPortString() with neither TCP nor UDP options = %q, want blank", got)
	}
}
