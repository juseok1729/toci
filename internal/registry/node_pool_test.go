package registry

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/containerengine"
)

func TestNodePoolResourceColumnsReadNodePoolRow(t *testing.T) {
	r := NewNodePoolResource(nil)
	row := Row{Raw: NodePoolRow{
		NodePoolSummary: containerengine.NodePoolSummary{
			Name:              common.String("pool-1"),
			LifecycleState:    containerengine.NodePoolLifecycleStateActive,
			KubernetesVersion: common.String("v1.28.2"),
			NodeShape:         common.String("VM.Standard.E4.Flex"),
			NodeShapeConfig:   &containerengine.NodeShapeConfig{Ocpus: common.Float32(2), MemoryInGBs: common.Float32(32)},
			NodeConfigDetails: &containerengine.NodePoolNodeConfigDetails{Size: common.Int(3)},
		},
	}}

	want := map[string]string{
		"NAME":        "pool-1",
		"STATE":       "Active",
		"K8S VERSION": "v1.28.2",
		"SHAPE":       "VM.Standard.E4.Flex",
		"OCPU":        "2.0",
		"MEM(GB)":     "32.0",
		"SIZE":        "3",
	}
	for _, col := range r.Columns() {
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}

// TestNodePoolResourceColumnsUnknownFieldsReadDash covers a fixed
// (non-".Flex") shape: NodeConfigDetails/NodeShapeConfig are nil since
// OCPU/memory/size aren't independently configurable for one.
func TestNodePoolResourceColumnsUnknownFieldsReadDash(t *testing.T) {
	r := NewNodePoolResource(nil)
	row := Row{Raw: NodePoolRow{NodePoolSummary: containerengine.NodePoolSummary{Name: common.String("pool-2")}}}

	for _, header := range []string{"SIZE", "OCPU", "MEM(GB)"} {
		for _, col := range r.Columns() {
			if col.Header == header {
				if got := col.Get(row); got != "-" {
					t.Errorf("%s with nil NodeConfigDetails/NodeShapeConfig = %q, want %q", header, got, "-")
				}
			}
		}
	}
}
