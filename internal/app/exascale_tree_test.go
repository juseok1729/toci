package app

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/database"
	"toci/internal/registry"
)

func TestExpandExascaleNodes(t *testing.T) {
	rows := []registry.Row{
		{ID: "c1", Name: "cluster-1", Raw: registry.ExadbVmClusterRow{
			Nodes: []database.DbNodeSummary{
				{Id: strPtr("n1"), Hostname: strPtr("node-1"), LifecycleState: database.DbNodeSummaryLifecycleStateAvailable, CpuCoreCount: intPtr(2), MemorySizeInGBs: intPtr(22)},
				{Id: strPtr("n2"), Hostname: strPtr("node-2"), LifecycleState: database.DbNodeSummaryLifecycleStateStopped},
			},
			NodeIPs: []string{"10.0.0.1", "10.0.0.2"},
		}},
		{ID: "c2", Name: "cluster-2", Raw: registry.ExadbVmClusterRow{}},
	}

	expanded := expandExascaleNodes(rows)
	wantOrder := []string{"c1", "c1:n1", "c1:n2", "c2"}
	if len(expanded) != len(wantOrder) {
		t.Fatalf("expandExascaleNodes() returned %d rows, want %d: %v", len(expanded), len(wantOrder), expanded)
	}
	for i, id := range wantOrder {
		if expanded[i].ID != id {
			t.Errorf("row %d: ID = %q, want %q", i, expanded[i].ID, id)
		}
	}

	glyphs := exascaleTreeGlyphs(expanded)
	if glyphs["c1:n1"] != treeChildMid {
		t.Errorf("c1:n1 glyph = %q, want mid-child %q (n2 follows it under the same cluster)", glyphs["c1:n1"], treeChildMid)
	}
	if glyphs["c1:n2"] != treeChildLast {
		t.Errorf("c1:n2 glyph = %q, want last-child %q (next row is cluster-2)", glyphs["c1:n2"], treeChildLast)
	}
	if _, ok := glyphs["c1"]; ok {
		t.Errorf("cluster row should not get a tree glyph")
	}

	cols := exascaleTreeColumns([]registry.Column{
		{Header: "NAME", Get: func(row registry.Row) string { return row.Raw.(registry.ExadbVmClusterRow).NodeStates[0] }},
		{Header: "STATE", Get: func(row registry.Row) string { return "cluster-state" }},
		{Header: "IP", Get: func(row registry.Row) string { return "cluster-ip" }},
		{Header: "OCPU", Get: func(row registry.Row) string { return "-" }},
		{Header: "MEM(GB)", Get: func(row registry.Row) string { return "cluster-mem" }},
	}, glyphs)
	if got := cols[0].Get(expanded[1]); got != treeChildMid+"node-1" {
		t.Errorf("node NAME = %q, want %q", got, treeChildMid+"node-1")
	}
	if got := cols[1].Get(expanded[1]); got != "Available" {
		t.Errorf("node STATE = %q, want %q", got, "Available")
	}
	if got := cols[2].Get(expanded[1]); got != "10.0.0.1" {
		t.Errorf("node IP = %q, want %q", got, "10.0.0.1")
	}
	if got := cols[3].Get(expanded[1]); got != "2" {
		t.Errorf("node OCPU = %q, want %q", got, "2")
	}
	if got := cols[4].Get(expanded[1]); got != "22" {
		t.Errorf("node MEM(GB) = %q, want %q", got, "22")
	}
	if got := cols[3].Get(expanded[2]); got != "-" {
		t.Errorf("node-2 OCPU (nil CpuCoreCount) = %q, want %q", got, "-")
	}

	filtered := filterOutExascaleNodes(expanded)
	if len(filtered) != 2 || filtered[0].ID != "c1" || filtered[1].ID != "c2" {
		t.Errorf("filterOutExascaleNodes() = %v, want only cluster rows", filtered)
	}
}
