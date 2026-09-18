package app

import (
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/mysql"
	"toci/internal/registry"
)

// mysqlNodeRow marks a synthetic Row (see expandMysqlNodes) as a HeatWave
// cluster node under a MySQL DB system row — the same shape as
// exascaleNodeRow/okeNodeRow, for the "g" node tree.
type mysqlNodeRow struct {
	Node mysql.HeatWaveNode // exported so the "d" detail YAML shows the node, not "{}"
}

// expandMysqlNodes inserts a synthetic child row for every HeatWave
// cluster node right after its DB system row; a DB system with no cluster
// attached (or whose GetHeatWaveCluster failed) stays a single row.
func expandMysqlNodes(rows []registry.Row) []registry.Row {
	out := make([]registry.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
		db, ok := row.Raw.(registry.MysqlDbSystemRow)
		if !ok || db.HeatWave == nil {
			continue
		}
		for i, n := range db.HeatWave.ClusterNodes {
			// NodeId is an opaque id, not a hostname — the node has no
			// display name of its own, so number them; the id is in the
			// row's detail ("d").
			out = append(out, registry.Row{ID: row.ID + ":" + deref(n.NodeId), Name: fmt.Sprintf("node-%d", i+1), Raw: mysqlNodeRow{Node: n}})
		}
	}
	return out
}

func isMysqlNode(row registry.Row) bool {
	_, ok := row.Raw.(mysqlNodeRow)
	return ok
}

// childTreeGlyphs assigns the ├─/└─ prefix to every child row (as isChild
// reports it) in a tree whose parents are real rows: a child is "last"
// when the next row is a parent (or the end of the list). Shared by the
// Exascale, OKE and MySQL node trees.
func childTreeGlyphs(rows []registry.Row, isChild func(registry.Row) bool) map[string]string {
	glyphs := make(map[string]string, len(rows))
	for i, row := range rows {
		if !isChild(row) {
			continue
		}
		if i == len(rows)-1 || !isChild(rows[i+1]) {
			glyphs[row.ID] = treeChildLast
		} else {
			glyphs[row.ID] = treeChildMid
		}
	}
	return glyphs
}

// mysqlTreeColumns decorates cols for the node tree: a DB system row
// renders exactly as Columns() defines it, a node row shows its number
// (tree-prefixed) and its own lifecycle state, blank elsewhere — a
// HeatWave node has no shape/version/endpoint of its own.
func mysqlTreeColumns(cols []registry.Column, glyphs map[string]string) []registry.Column {
	out := make([]registry.Column, len(cols))
	for i, c := range cols {
		i, c := i, c
		out[i] = registry.Column{Header: c.Header, Width: c.Width, Get: func(row registry.Row) string {
			n, isNode := row.Raw.(mysqlNodeRow)
			if !isNode {
				return c.Get(row)
			}
			switch {
			case c.Header == "NAME":
				return glyphs[row.ID] + row.Name
			case c.Header == "STATE":
				return registry.StateLabel(n.Node.LifecycleState)
			case i == 0:
				return glyphs[row.ID]
			default:
				return ""
			}
		}}
	}
	return out
}

// filterOutMysqlNodes strips the synthetic node rows (CSV export).
func filterOutMysqlNodes(rows []registry.Row) []registry.Row {
	out := make([]registry.Row, 0, len(rows))
	for _, row := range rows {
		if !isMysqlNode(row) {
			out = append(out, row)
		}
	}
	return out
}

// mysqlDetail is what Enter shows for a MySQL DB system: the connection
// info first (every endpoint with its ports and hostname, plus ready-made
// mysql/mysqlsh command lines), the HeatWave cluster if one is attached,
// then the usual full YAML dump below a separator.
func mysqlDetail(row registry.Row) string {
	db, ok := row.Raw.(registry.MysqlDbSystemRow)
	if !ok {
		return renderDetail(row)
	}
	var b strings.Builder
	b.WriteString("CONNECTION\n")
	if len(db.Endpoints) == 0 {
		b.WriteString("  (no endpoints yet)\n")
	}
	for _, ep := range db.Endpoints {
		modes := make([]string, len(ep.Modes))
		for i, m := range ep.Modes {
			modes[i] = string(m)
		}
		fmt.Fprintf(&b, "  %-14s %s:%s  mysqlx %s  [%s %s]\n",
			strings.ToLower(string(ep.ResourceType)), deref(ep.IpAddress), intStr(ep.Port), intStr(ep.PortX),
			strings.Join(modes, "/"), registry.StateLabel(ep.Status))
		if ep.Hostname != nil {
			fmt.Fprintf(&b, "  %-14s %s\n", "", *ep.Hostname)
		}
	}
	if ep, ok := db.PrimaryEndpoint(); ok && ep.IpAddress != nil {
		fmt.Fprintf(&b, "\n  mysql   -h %s -P %s -u <user> -p\n", *ep.IpAddress, intStr(ep.Port))
		fmt.Fprintf(&b, "  mysqlsh <user>@%s:%s\n", *ep.IpAddress, intStr(ep.PortX))
	}
	if hw := db.HeatWave; hw != nil {
		fmt.Fprintf(&b, "\nHEATWAVE CLUSTER\n  shape %s  size %s  state %s\n",
			deref(hw.ShapeName), intStr(hw.ClusterSize), registry.StateLabel(hw.LifecycleState))
		for i, n := range hw.ClusterNodes {
			fmt.Fprintf(&b, "  node-%d  %-12s %s\n", i+1, registry.StateLabel(n.LifecycleState), deref(n.NodeId))
		}
	}
	b.WriteString("\n---\n")
	b.WriteString(renderDetail(row))
	return b.String()
}

func intStr(n *int) string {
	if n == nil {
		return "-"
	}
	return fmt.Sprint(*n)
}
