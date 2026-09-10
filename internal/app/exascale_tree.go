package app

import (
	"strconv"

	"github.com/oracle/oci-go-sdk/v65/database"
	"toci/internal/registry"
)

// exascaleNodeRow marks a synthetic Row (see expandExascaleNodes) as a DB
// node under an Exadata VM cluster (Exascale) row, rather than the cluster
// itself — every Get closure exascaleTreeColumns builds checks for it
// before touching row.Raw, since it's never a registry.ExadbVmClusterRow.
type exascaleNodeRow struct {
	node database.DbNodeSummary
	ip   string
}

// expandExascaleNodes inserts a synthetic child row for every DB node right
// after its Exadata VM cluster row — tree-style, like groupRowsByVcn's VCN
// grouping, except the cluster row is the real one (no synthetic header)
// and its children follow it directly instead of preceding a group.
func expandExascaleNodes(rows []registry.Row) []registry.Row {
	out := make([]registry.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
		cluster, ok := row.Raw.(registry.ExadbVmClusterRow)
		if !ok {
			continue
		}
		for i, n := range cluster.Nodes {
			var ip string
			if i < len(cluster.NodeIPs) {
				ip = cluster.NodeIPs[i]
			}
			out = append(out, registry.Row{ID: row.ID + ":" + deref(n.Id), Name: deref(n.Hostname), Raw: exascaleNodeRow{node: n, ip: ip}})
		}
	}
	return out
}

// exascaleTreeGlyphs mirrors treeGlyphs, but the group boundary is a real
// cluster row (not a synthetic header) — so a node row is "last" when the
// next row is either another cluster row or the end of the list.
func exascaleTreeGlyphs(rows []registry.Row) map[string]string {
	glyphs := make(map[string]string, len(rows))
	for i, row := range rows {
		if _, isNode := row.Raw.(exascaleNodeRow); !isNode {
			continue
		}
		last := i == len(rows)-1
		if !last {
			_, nextIsNode := rows[i+1].Raw.(exascaleNodeRow)
			last = !nextIsNode
		}
		if last {
			glyphs[row.ID] = treeChildLast
		} else {
			glyphs[row.ID] = treeChildMid
		}
	}
	return glyphs
}

// exascaleTreeColumns decorates cols for the node tree: a cluster row
// renders exactly as Columns() defines it (ECPU/MEM(GB)/IP there are
// cluster-wide totals/SCAN IPs), and a node row renders its hostname
// (tree-prefixed) in the first column, its own lifecycle state, host IP,
// OCPU count and memory, and blanks everywhere else — the cluster-level
// columns (shape, license...) don't apply to an individual node.
func exascaleTreeColumns(cols []registry.Column, glyphs map[string]string) []registry.Column {
	out := make([]registry.Column, len(cols))
	for i, c := range cols {
		i, c := i, c
		out[i] = registry.Column{Header: c.Header, Width: c.Width, Get: func(row registry.Row) string {
			n, isNode := row.Raw.(exascaleNodeRow)
			if !isNode {
				return c.Get(row)
			}
			switch c.Header {
			case "NAME":
				return glyphs[row.ID] + deref(n.node.Hostname)
			case "STATE":
				return registry.StateLabel(n.node.LifecycleState)
			case "IP":
				if n.ip == "" {
					return "-"
				}
				return n.ip
			case "OCPU":
				if n.node.CpuCoreCount == nil {
					return "-"
				}
				return strconv.Itoa(*n.node.CpuCoreCount)
			case "MEM(GB)":
				if n.node.MemorySizeInGBs == nil {
					return "-"
				}
				return strconv.Itoa(*n.node.MemorySizeInGBs)
			default:
				if i == 0 {
					return glyphs[row.ID]
				}
				return ""
			}
		}}
	}
	return out
}

// filterOutExascaleNodes strips the synthetic node rows expandExascaleNodes
// inserted — e.g. before CSV export, whose columns don't recognize
// exascaleNodeRow, and which wants real cluster rows only anyway.
func filterOutExascaleNodes(rows []registry.Row) []registry.Row {
	out := make([]registry.Row, 0, len(rows))
	for _, row := range rows {
		if _, isNode := row.Raw.(exascaleNodeRow); isNode {
			continue
		}
		out = append(out, row)
	}
	return out
}
