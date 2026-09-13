package app

import (
	"sort"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/registry"
)

// vcnGroupHeader marks a synthetic Row (see groupRowsByVcn) as a VCN group
// header rather than a real subnet — every Get closure treeColumns builds
// checks for it before touching row.Raw, since it's never a core.Subnet.
type vcnGroupHeader struct {
	name string
	cidr string
}

const (
	treeChildMid  = "  ├─ "
	treeChildLast = "  └─ "
	treeGroupIcon = "▾ "
)

// vcnInfo is what fetchVcnNames resolves per VCN — the Name and Cidr the
// group header row shows for it once loaded.
type vcnInfo struct {
	Name string
	Cidr string
}

// subnetVcnID returns a subnet row's VcnId, or "" if row isn't a subnet or
// has none.
func subnetVcnID(row registry.Row) string {
	sn, ok := row.Raw.(core.Subnet)
	if !ok {
		return ""
	}
	return deref(sn.VcnId)
}

// vcnLabel returns a subnet row's VCN name for grouping/sorting, falling
// back to the VCN's OCID when fetchVcnNames hasn't resolved it yet.
func vcnLabel(row registry.Row, infos map[string]vcnInfo) string {
	id := subnetVcnID(row)
	if id == "" {
		return ""
	}
	if info, ok := infos[id]; ok && info.Name != "" {
		return info.Name
	}
	return id
}

// groupRowsByVcn sorts subnet rows by their VCN and inserts a synthetic
// header row (Raw: vcnGroupHeader, carrying the VCN's own CIDR alongside
// its name) before each group — tree-style, like the AWS console's "DB
// identifier" grouping. Real rows are returned unchanged; treeColumns adds
// the tree-connector glyph at render time (keyed by row.ID, via
// treeGlyphs) so this stays a pure row-ordering step.
func groupRowsByVcn(rows []registry.Row, infos map[string]vcnInfo) []registry.Row {
	sorted := make([]registry.Row, len(rows))
	copy(sorted, rows)
	sort.SliceStable(sorted, func(i, j int) bool {
		return vcnLabel(sorted[i], infos) < vcnLabel(sorted[j], infos)
	})

	out := make([]registry.Row, 0, len(sorted)+len(infos))
	var groupLabel string
	for i, row := range sorted {
		label := vcnLabel(row, infos)
		if i == 0 || label != groupLabel {
			cidr := infos[subnetVcnID(row)].Cidr
			out = append(out, registry.Row{ID: "vcn-header:" + label, Name: label, Raw: vcnGroupHeader{name: label, cidr: cidr}})
			groupLabel = label
		}
		out = append(out, row)
	}
	return out
}

// treeGlyphs maps each child row's ID to its tree-connector prefix ("├─ "
// for a middle child, "└─ " for the last child in its group) — built from
// groupRowsByVcn's output so treeColumns can prefix the first column
// without threading extra state through registry.Row.
func treeGlyphs(rows []registry.Row) map[string]string {
	glyphs := make(map[string]string, len(rows))
	for i, row := range rows {
		if _, isHeader := row.Raw.(vcnGroupHeader); isHeader {
			continue
		}
		last := i == len(rows)-1
		if !last {
			_, nextIsHeader := rows[i+1].Raw.(vcnGroupHeader)
			last = nextIsHeader
		}
		if last {
			glyphs[row.ID] = treeChildLast
		} else {
			glyphs[row.ID] = treeChildMid
		}
	}
	return glyphs
}

// treeColumns decorates cols for tree display: a header row renders the VCN
// name (with its group icon) in the first column, its own CIDR/IP RANGE in
// any column titled that (skipping the real Get closures entirely, since a
// vcnGroupHeader Raw isn't the SDK type they expect — real columns keyed
// off cols[i].Header the same way colorizeState does), and blank elsewhere;
// a real row gets a tree-connector prefix on the first column. Column count
// is unchanged from cols, so this can't desync from the row cells already
// sitting in bubbles/table (see displayColumns' doc comment).
func treeColumns(cols []registry.Column, glyphs map[string]string) []registry.Column {
	out := make([]registry.Column, len(cols))
	for i, c := range cols {
		i, c := i, c
		out[i] = registry.Column{Header: c.Header, Width: c.Width, Get: func(row registry.Row) string {
			if h, ok := row.Raw.(vcnGroupHeader); ok {
				switch {
				case c.Header == "CIDR":
					return h.cidr
				case c.Header == "IP RANGE":
					return registry.CidrRange(h.cidr)
				case i == 0:
					return treeGroupIcon + h.name
				default:
					return ""
				}
			}
			if i == 0 {
				return glyphs[row.ID] + c.Get(row)
			}
			return c.Get(row)
		}}
	}
	return out
}

// filterOutGroupHeaders strips the synthetic VCN header rows groupRowsByVcn
// inserted — e.g. before CSV export, which wants real resource rows only.
func filterOutGroupHeaders(rows []registry.Row) []registry.Row {
	out := make([]registry.Row, 0, len(rows))
	for _, row := range rows {
		if _, isHeader := row.Raw.(vcnGroupHeader); isHeader {
			continue
		}
		out = append(out, row)
	}
	return out
}
