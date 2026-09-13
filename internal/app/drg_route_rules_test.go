package app

import (
	"reflect"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/registry"
)

// TestDrgRouteRuleRecords pins down drgRouteRuleRecords' two branches: a
// blackhole route shows "BLACKHOLE" regardless of NextHopDrgAttachmentId,
// and a normal route resolves its next hop's OCID to a name when
// attachmentNames has one, falling back to the raw OCID otherwise (deleted
// attachment, or the ListDrgAttachments lookup itself failed).
func TestDrgRouteRuleRecords(t *testing.T) {
	rules := []core.DrgRouteRule{
		{
			Destination:            strPtr("10.0.0.0/16"),
			NextHopDrgAttachmentId: strPtr("ocid1.drgattachment.oc1..known"),
			RouteType:              core.DrgRouteRuleRouteTypeStatic,
			RouteProvenance:        core.DrgRouteRuleRouteProvenanceStatic,
		},
		{
			Destination:            strPtr("192.168.0.0/16"),
			NextHopDrgAttachmentId: strPtr("ocid1.drgattachment.oc1..unknown"),
			RouteType:              core.DrgRouteRuleRouteTypeStatic,
			RouteProvenance:        core.DrgRouteRuleRouteProvenanceStatic,
		},
		{
			Destination:            strPtr("0.0.0.0/0"),
			NextHopDrgAttachmentId: strPtr("ocid1.drgattachment.oc1..known"),
			RouteType:              core.DrgRouteRuleRouteTypeStatic,
			RouteProvenance:        core.DrgRouteRuleRouteProvenanceStatic,
			IsBlackhole:            boolPtr(true),
		},
	}
	names := map[string]string{"ocid1.drgattachment.oc1..known": "hub-vcn-attachment"}

	got := drgRouteRuleRecords(rules, names)
	want := [][]string{
		{"10.0.0.0/16", "hub-vcn-attachment", "STATIC", "STATIC"},
		{"192.168.0.0/16", "ocid1.drgattachment.oc1..unknown", "STATIC", "STATIC"},
		{"0.0.0.0/0", "BLACKHOLE", "STATIC", "STATIC"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("drgRouteRuleRecords() = %v, want %v", got, want)
	}
}

// TestBuildDrgRouteRulesCmdRejectsWrongRowType mirrors
// TestBuildNsgRulesCmdRejectsWrongRowType — "v" is gated on the current
// resource being "drg-route-table" by updateTable, but the type assertion
// should still fail closed (nil cmd) rather than panic on a mismatched row.
func TestBuildDrgRouteRulesCmdRejectsWrongRowType(t *testing.T) {
	m := Model{}
	row := registry.Row{ID: "vcn1", Name: "my-vcn", Raw: core.Vcn{}}
	if cmd := m.buildDrgRouteRulesCmd(row); cmd != nil {
		t.Error("buildDrgRouteRulesCmd on a non-DrgRouteTable row should return a nil cmd")
	}
}
