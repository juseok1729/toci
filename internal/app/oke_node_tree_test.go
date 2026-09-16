package app

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/containerengine"
	"toci/internal/registry"
)

func TestExpandOkeNodes(t *testing.T) {
	rows := []registry.Row{
		{ID: "p1", Name: "pool-1", Raw: registry.NodePoolRow{
			NodePoolSummary: containerengine.NodePoolSummary{
				NodeShapeConfig: &containerengine.NodeShapeConfig{Ocpus: common.Float32(2), MemoryInGBs: common.Float32(32)},
			},
			Nodes: []containerengine.Node{
				{Id: strPtr("n1"), Name: strPtr("node-1"), LifecycleState: containerengine.NodeLifecycleStateActive, KubernetesVersion: strPtr("v1.28.2")},
				{Id: strPtr("n2"), Name: strPtr("node-2"), LifecycleState: containerengine.NodeLifecycleStateCreating},
			},
		}},
		{ID: "p2", Name: "pool-2", Raw: registry.NodePoolRow{}},
	}

	expanded := expandOkeNodes(rows)
	wantOrder := []string{"p1", "p1:n1", "p1:n2", "p2"}
	if len(expanded) != len(wantOrder) {
		t.Fatalf("expandOkeNodes() returned %d rows, want %d: %v", len(expanded), len(wantOrder), expanded)
	}
	for i, id := range wantOrder {
		if expanded[i].ID != id {
			t.Errorf("row %d: ID = %q, want %q", i, expanded[i].ID, id)
		}
	}

	glyphs := okeTreeGlyphs(expanded)
	if glyphs["p1:n1"] != treeChildMid {
		t.Errorf("p1:n1 glyph = %q, want mid-child %q (n2 follows it under the same pool)", glyphs["p1:n1"], treeChildMid)
	}
	if glyphs["p1:n2"] != treeChildLast {
		t.Errorf("p1:n2 glyph = %q, want last-child %q (next row is pool-2)", glyphs["p1:n2"], treeChildLast)
	}
	if _, ok := glyphs["p1"]; ok {
		t.Errorf("pool row should not get a tree glyph")
	}

	cols := okeTreeColumns([]registry.Column{
		{Header: "NAME", Get: func(row registry.Row) string { return "pool-name" }},
		{Header: "STATE", Get: func(row registry.Row) string { return "pool-state" }},
		{Header: "K8S VERSION", Get: func(row registry.Row) string { return "pool-version" }},
		{Header: "SHAPE", Get: func(row registry.Row) string { return "pool-shape" }},
		{Header: "OCPU", Get: func(row registry.Row) string { return "pool-ocpu" }},
		{Header: "MEM(GB)", Get: func(row registry.Row) string { return "pool-mem" }},
		{Header: "SIZE", Get: func(row registry.Row) string { return "3" }},
	}, glyphs)
	if got := cols[0].Get(expanded[1]); got != treeChildMid+"node-1" {
		t.Errorf("node NAME = %q, want %q", got, treeChildMid+"node-1")
	}
	if got := cols[1].Get(expanded[1]); got != "Active" {
		t.Errorf("node STATE = %q, want %q", got, "Active")
	}
	if got := cols[2].Get(expanded[1]); got != "v1.28.2" {
		t.Errorf("node K8S VERSION = %q, want %q", got, "v1.28.2")
	}
	if got := cols[3].Get(expanded[1]); got != "" {
		t.Errorf("node SHAPE = %q, want blank", got)
	}
	// OCPU/MEM(GB) are the pool's own NodeShapeConfig, copied down to every
	// node under it (see okeNodeRow's doc) — unlike SHAPE/SIZE, not blank.
	if got := cols[4].Get(expanded[1]); got != "2.0" {
		t.Errorf("node OCPU = %q, want %q (the pool's own shape config)", got, "2.0")
	}
	if got := cols[5].Get(expanded[1]); got != "32.0" {
		t.Errorf("node MEM(GB) = %q, want %q (the pool's own shape config)", got, "32.0")
	}
	if got := cols[6].Get(expanded[1]); got != "" {
		t.Errorf("node SIZE = %q, want blank", got)
	}
	if got := cols[0].Get(expanded[0]); got != "pool-name" {
		t.Errorf("pool row NAME = %q, want passthrough %q", got, "pool-name")
	}

	filtered := filterOutOkeNodes(expanded)
	if len(filtered) != 2 || filtered[0].ID != "p1" || filtered[1].ID != "p2" {
		t.Errorf("filterOutOkeNodes() = %v, want only pool rows", filtered)
	}
}
