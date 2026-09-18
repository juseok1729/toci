package app

import (
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/mysql"
	"toci/internal/registry"
)

func mysqlTestRows() []registry.Row {
	yes := true
	port, portX := 3306, 33060
	return []registry.Row{
		{ID: "db1", Name: "hw-db", Raw: registry.MysqlDbSystemRow{
			DbSystemSummary: mysql.DbSystemSummary{
				DisplayName:               strPtr("hw-db"),
				IsHeatWaveClusterAttached: &yes,
				HeatWaveCluster:           &mysql.HeatWaveClusterSummary{ClusterSize: intPtr(2), LifecycleState: mysql.HeatWaveClusterLifecycleStateActive},
				Endpoints: []mysql.DbSystemEndpoint{
					{IpAddress: strPtr("10.0.1.5"), Port: &port, PortX: &portX, Hostname: strPtr("hw-db.sub.vcn.oraclevcn.com"),
						ResourceType: mysql.DbSystemEndpointResourceTypeDbsystem, Modes: []mysql.DbSystemEndpointModesEnum{mysql.DbSystemEndpointModesRead, mysql.DbSystemEndpointModesWrite}, Status: mysql.DbSystemEndpointStatusActive},
				},
			},
			HeatWave: &mysql.HeatWaveCluster{ShapeName: strPtr("MySQL.HeatWave.VM.Standard"), ClusterSize: intPtr(2), LifecycleState: mysql.HeatWaveClusterLifecycleStateActive, ClusterNodes: []mysql.HeatWaveNode{
				{NodeId: strPtr("n1"), LifecycleState: mysql.HeatWaveNodeLifecycleStateActive},
				{NodeId: strPtr("n2"), LifecycleState: mysql.HeatWaveNodeLifecycleStateInactive},
			}},
		}},
		{ID: "db2", Name: "plain-db", Raw: registry.MysqlDbSystemRow{DbSystemSummary: mysql.DbSystemSummary{DisplayName: strPtr("plain-db")}}},
	}
}

func TestExpandMysqlNodes(t *testing.T) {
	expanded := expandMysqlNodes(mysqlTestRows())
	wantOrder := []string{"db1", "db1:n1", "db1:n2", "db2"}
	if len(expanded) != len(wantOrder) {
		t.Fatalf("expandMysqlNodes() returned %d rows, want %d", len(expanded), len(wantOrder))
	}
	for i, id := range wantOrder {
		if expanded[i].ID != id {
			t.Errorf("row %d: ID = %q, want %q", i, expanded[i].ID, id)
		}
	}

	glyphs := childTreeGlyphs(expanded, isMysqlNode)
	if glyphs["db1:n1"] != treeChildMid || glyphs["db1:n2"] != treeChildLast {
		t.Errorf("glyphs = %v, want n1 mid / n2 last", glyphs)
	}
	if _, ok := glyphs["db1"]; ok {
		t.Errorf("DB system row should not get a tree glyph")
	}

	cols := mysqlTreeColumns(registry.NewMysqlDbSystemResource(nil).Columns(), glyphs)
	get := func(header string, row registry.Row) string {
		for _, c := range cols {
			if c.Header == header {
				return c.Get(row)
			}
		}
		t.Fatalf("no %s column", header)
		return ""
	}
	if got := get("NAME", expanded[1]); got != treeChildMid+"node-1" {
		t.Errorf("node NAME = %q", got)
	}
	if got := get("STATE", expanded[2]); got != "Inactive" {
		t.Errorf("node STATE = %q, want Inactive", got)
	}
	if got := get("HEATWAVE", expanded[1]); got != "" {
		t.Errorf("node HEATWAVE = %q, want blank", got)
	}
	if got := get("HEATWAVE", expanded[0]); got != "Active (2)" {
		t.Errorf("cluster HEATWAVE = %q, want %q", got, "Active (2)")
	}
	if got := get("HEATWAVE", expanded[3]); got != "-" {
		t.Errorf("plain DB HEATWAVE = %q, want -", got)
	}
	if got := get("ENDPOINT", expanded[0]); got != "10.0.1.5:3306" {
		t.Errorf("ENDPOINT = %q", got)
	}
	if got := filterOutMysqlNodes(expanded); len(got) != 2 {
		t.Errorf("filterOutMysqlNodes left %d rows, want 2", len(got))
	}
}

func TestMysqlDetailShowsConnectionInfo(t *testing.T) {
	out := mysqlDetail(mysqlTestRows()[0])
	for _, want := range []string{
		"CONNECTION",
		"dbsystem       10.0.1.5:3306  mysqlx 33060  [READ/WRITE Active]",
		"hw-db.sub.vcn.oraclevcn.com",
		"mysql   -h 10.0.1.5 -P 3306 -u <user> -p",
		"mysqlsh <user>@10.0.1.5:33060",
		"HEATWAVE CLUSTER",
		"shape MySQL.HeatWave.VM.Standard  size 2  state Active",
		"node-2  Inactive     n2",
		"\n---\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q:\n%s", want, out)
		}
	}
	// A node row (Enter while the tree is expanded) gets the plain YAML.
	node := expandMysqlNodes(mysqlTestRows())[1]
	if got := mysqlDetail(node); strings.Contains(got, "CONNECTION") || !strings.Contains(got, "n1") {
		t.Errorf("node detail = %q, want plain YAML of the node", got)
	}
}
