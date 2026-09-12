package app

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/database"

	"toci/internal/registry"
)

func TestRowClipboardID(t *testing.T) {
	nodeID := "ocid1.dbnode.oc1..realnode"

	cases := []struct {
		name   string
		row    registry.Row
		wantID string
		wantOK bool
	}{
		{"plain row copies its own ID", registry.Row{ID: "ocid1.vcn.oc1..abc"}, "ocid1.vcn.oc1..abc", true},
		{"vcn group header has nothing to copy", registry.Row{ID: "group", Raw: vcnGroupHeader{name: "vcn-1"}}, "", false},
		{
			"exascale db node copies the node's real OCID, not the composite table key",
			registry.Row{ID: "ocid1.cluster..xyz:ocid1.dbnode..realnode", Raw: exascaleNodeRow{node: database.DbNodeSummary{Id: &nodeID}}},
			nodeID, true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := rowClipboardID(c.row)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && id != c.wantID {
				t.Errorf("id = %q, want %q", id, c.wantID)
			}
		})
	}
}
