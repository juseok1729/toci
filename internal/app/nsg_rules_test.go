package app

import (
	"reflect"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/registry"
)

// TestNsgRuleRecords pins down the one bit of logic nsgRuleRecords adds on
// top of the shared protocolName/portsString/yesNo helpers: an ingress
// rule's endpoint comes from Source, an egress rule's from Destination —
// core.SecurityRule holds both fields, only one of which is meaningful per
// direction.
func TestNsgRuleRecords(t *testing.T) {
	rules := []core.SecurityRule{
		{
			Direction:   core.SecurityRuleDirectionIngress,
			Protocol:    strPtr("6"),
			Source:      strPtr("10.0.0.0/24"),
			Destination: strPtr("should-be-ignored"),
			TcpOptions: &core.TcpOptions{
				DestinationPortRange: &core.PortRange{Min: intPtr(22), Max: intPtr(22)},
			},
			Description: strPtr("ssh in"),
		},
		{
			Direction:   core.SecurityRuleDirectionEgress,
			Protocol:    strPtr("all"),
			Destination: strPtr("0.0.0.0/0"),
			IsStateless: boolPtr(true),
		},
	}

	got := nsgRuleRecords(rules)
	want := [][]string{
		{"INGRESS", "TCP", "10.0.0.0/24", "22", "no", "ssh in"},
		{"EGRESS", "ALL", "0.0.0.0/0", "ALL", "yes", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nsgRuleRecords() = %v, want %v", got, want)
	}
}

// TestBuildNsgRulesCmdRejectsWrongRowType guards the type assertion in
// buildNsgRulesCmd — "v" is gated on the current resource being "nsg" by
// updateTable, but a stale/mismatched row.Raw should still fail closed
// (nil cmd) rather than panic.
func TestBuildNsgRulesCmdRejectsWrongRowType(t *testing.T) {
	m := Model{}
	row := registry.Row{ID: "vcn1", Name: "my-vcn", Raw: core.Vcn{}}
	if cmd := m.buildNsgRulesCmd(row); cmd != nil {
		t.Error("buildNsgRulesCmd on a non-NSG row should return a nil cmd")
	}
}
