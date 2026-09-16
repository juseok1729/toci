package registry

import (
	"context"
	"sort"
	"time"
)

// RecentWindow is how far back RecentResource looks for recently *created*
// resources — same window as the in-table blink highlight (toci/internal/
// app's recentRowWindow), just applied across every resource kind instead
// of one already-loaded table. Based on Row.TimeCreated only: OCI's list
// APIs don't expose a last-modified timestamp, so this can't (and doesn't
// try to) catch updates to existing resources, only new ones.
const RecentWindow = 3 * 24 * time.Hour

// recentExcludedKeys are resource kinds RecentResource's fan-out skips:
// Compartments rarely change and aren't the kind of thing this view is
// for; Internet/NAT/Service Gateway are already covered by the combined
// "gateway" kind (including them too would triple-count every gateway);
// DRG Route Table/Distribution need a DrgID just to list at all (see
// drgIDRequiredResourceKeys in internal/app) so an unscoped List call on
// them would just error.
var recentExcludedKeys = map[string]bool{
	"compartment": true, "igw": true, "nat-gateway": true, "service-gateway": true,
	"drg-route-table": true, "drg-route-distribution": true,
}

// recentRow is RecentResource's normalized row shape — every source
// resource kind's own Raw type collapsed down to what's actually shown,
// tagged with which kind it came from (same reasoning as gatewayRow).
type recentRow struct {
	Kind  string
	Name  string
	State string
}

// RecentResource is a synthetic, read-only view: rather than mapping to
// one OCI API, it fans out across every other registered resource kind
// (minus recentExcludedKeys) and merges whatever each one returns within
// RecentWindow into a single list sorted newest-first — a way to see
// what's new without checking every resource kind by hand. Creation-only
// (Row.TimeCreated) — OCI's list APIs don't expose a last-modified
// timestamp for any resource kind this app browses, so there's no
// equivalent "recently updated" view to build here.
type RecentResource struct {
	sources []Resource
}

// NewRecentResource takes every already-constructed resource (registry.
// All's own list) and keeps the ones it fans out across — done here rather
// than by the caller so All() doesn't need to know recentExcludedKeys.
func NewRecentResource(all []Resource) *RecentResource {
	sources := make([]Resource, 0, len(all))
	for _, r := range all {
		if !recentExcludedKeys[r.Key()] {
			sources = append(sources, r)
		}
	}
	return &RecentResource{sources: sources}
}

func (r *RecentResource) Key() string   { return "recent" }
func (r *RecentResource) Label() string { return "Recently Created" }

func (r *RecentResource) Columns() []Column {
	return []Column{
		{Header: "KIND", Width: 18, Get: func(row Row) string { return row.Raw.(recentRow).Kind }},
		{Header: "NAME", Width: 30, Get: func(row Row) string { return row.Raw.(recentRow).Name }},
		{Header: "STATE", Width: 12, Get: func(row Row) string { return row.Raw.(recentRow).State }},
		{Header: "CREATED", Width: 17, Get: func(row Row) string { return row.TimeCreated.Local().Format("2006-01-02 15:04") }},
	}
}

// columnValue runs one of r's own Columns() Get closures against row by
// header name — used below to read a source resource's "STATE" without
// RecentResource needing to know its concrete Raw type (most resource
// kinds have one; VCNs/Subnets/etc. don't expose a lifecycle state at all,
// so this just reads back "" for those).
func columnValue(r Resource, header string, row Row) string {
	for _, c := range r.Columns() {
		if c.Header == header {
			return c.Get(row)
		}
	}
	return ""
}

// List fans out across every source resource kind, draining each one's own
// pagination fully — same reasoning as GatewayResource.List: there's no
// single page token that could represent "which source, and which page of
// it" — keeps only rows created within RecentWindow, and sorts newest
// first. One source erroring (e.g. no read permission on that kind in this
// compartment) is skipped rather than failing the whole view.
func (r *RecentResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	cutoff := time.Now().Add(-RecentWindow)
	var rows []Row
	for _, src := range r.sources {
		p := ""
		for {
			items, next, err := src.List(ctx, s, p)
			if err != nil {
				break
			}
			for _, row := range items {
				if row.TimeCreated.After(cutoff) {
					rows = append(rows, Row{
						ID: row.ID, Name: row.Name, TimeCreated: row.TimeCreated,
						Raw: recentRow{Kind: src.Label(), Name: row.Name, State: columnValue(src, "STATE", row)},
					})
				}
			}
			if next == "" {
				break
			}
			p = next
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TimeCreated.After(rows[j].TimeCreated) })
	return rows, "", nil
}
