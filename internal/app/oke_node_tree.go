package app

import (
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/containerengine"
	"toci/internal/registry"
)

// okeNodeRow marks a synthetic Row (see expandOkeNodes) as a worker node
// under a Node Pool row, rather than the pool itself — every Get closure
// okeTreeColumns builds checks for it before touching row.Raw, since it's
// never a registry.NodePoolRow. ocpu/mem come from the *pool's* own
// NodeShapeConfig, not the Node struct (which carries neither) — every
// node under one pool shares the same shape, so this is a straight copy
// down to each of the pool's own children, not a per-node fetch.
type okeNodeRow struct {
	node containerengine.Node
	ocpu *float32
	mem  *float32
}

// okeFloatString mirrors the registry package's own floatString (unexported
// there) for NodeShapeConfig's Ocpus/MemoryInGBs — nil (a fixed, non-
// ".Flex" shape) reads as "-", same as every other unknown-figure column.
func okeFloatString(v *float32) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.1f", *v)
}

// expandOkeNodes inserts a synthetic child row for every worker node right
// after its Node Pool row — tree-style, like expandExascaleNodes, the
// pool row is the real one (no synthetic header) and its nodes follow it
// directly instead of preceding a group.
func expandOkeNodes(rows []registry.Row) []registry.Row {
	out := make([]registry.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
		pool, ok := row.Raw.(registry.NodePoolRow)
		if !ok {
			continue
		}
		var ocpu, mem *float32
		if pool.NodeShapeConfig != nil {
			ocpu = pool.NodeShapeConfig.Ocpus
			mem = pool.NodeShapeConfig.MemoryInGBs
		}
		for _, n := range pool.Nodes {
			out = append(out, registry.Row{ID: row.ID + ":" + deref(n.Id), Name: deref(n.Name), Raw: okeNodeRow{node: n, ocpu: ocpu, mem: mem}})
		}
	}
	return out
}

// okeTreeGlyphs mirrors exascaleTreeGlyphs: the group boundary is a real
// pool row (not a synthetic header), so a node row is "last" when the next
// row is either another pool row or the end of the list.
func okeTreeGlyphs(rows []registry.Row) map[string]string {
	glyphs := make(map[string]string, len(rows))
	for i, row := range rows {
		if _, isNode := row.Raw.(okeNodeRow); !isNode {
			continue
		}
		last := i == len(rows)-1
		if !last {
			_, nextIsNode := rows[i+1].Raw.(okeNodeRow)
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

// okeTreeColumns decorates cols for the node tree: a pool row renders
// exactly as NodePoolResource.Columns() defines it, and a node row renders
// its own name (tree-prefixed), lifecycle state, Kubernetes version, and
// its pool's OCPU/MEM(GB) shape (same value the pool row itself shows,
// copied down — see okeNodeRow's own doc). SHAPE/SIZE stay pool-only:
// SHAPE (the shape *name*, e.g. "VM.Standard.E4.Flex") isn't itself a
// per-node figure the way OCPU/MEM(GB) are, and SIZE (pool-wide node
// count) makes no sense repeated on each of those very nodes.
func okeTreeColumns(cols []registry.Column, glyphs map[string]string) []registry.Column {
	out := make([]registry.Column, len(cols))
	for i, c := range cols {
		i, c := i, c
		out[i] = registry.Column{Header: c.Header, Width: c.Width, Get: func(row registry.Row) string {
			n, isNode := row.Raw.(okeNodeRow)
			if !isNode {
				return c.Get(row)
			}
			switch c.Header {
			case "NAME":
				return glyphs[row.ID] + deref(n.node.Name)
			case "STATE":
				return registry.StateLabel(n.node.LifecycleState)
			case "K8S VERSION":
				return deref(n.node.KubernetesVersion)
			case "OCPU":
				return okeFloatString(n.ocpu)
			case "MEM(GB)":
				return okeFloatString(n.mem)
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

// filterOutOkeNodes strips the synthetic node rows expandOkeNodes
// inserted — e.g. before CSV export, whose columns don't recognize
// okeNodeRow, and which wants real pool rows only anyway.
func filterOutOkeNodes(rows []registry.Row) []registry.Row {
	out := make([]registry.Row, 0, len(rows))
	for _, row := range rows {
		if _, isNode := row.Raw.(okeNodeRow); isNode {
			continue
		}
		out = append(out, row)
	}
	return out
}
