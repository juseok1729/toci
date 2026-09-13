package registry

import (
	"context"
	"strings"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

type RouteTableResource struct {
	factory *clients.Factory
}

func NewRouteTableResource(f *clients.Factory) *RouteTableResource {
	return &RouteTableResource{factory: f}
}

func (r *RouteTableResource) Key() string   { return "route-table" }
func (r *RouteTableResource) Label() string { return "Route Tables" }

// routeTableAccessLabel infers Public/Private the same way OCI's own
// console framing implies it: a route table isn't public/private itself
// (that's a Subnet attribute, see subnetAccessLabel) — it earns the label
// from routing 0.0.0.0/0-style traffic out through an Internet Gateway,
// vs. only NAT/Service Gateway/DRG/local routes.
func routeTableAccessLabel(rt core.RouteTable) string {
	for _, r := range rt.RouteRules {
		if routeRuleTargetsInternetGateway(deref(r.NetworkEntityId)) {
			return "Public"
		}
	}
	return "Private"
}

// routeRuleTargetsInternetGateway checks a route rule's NetworkEntityId
// OCID for the "internetgateway" resource-type segment
// ("ocid1.internetgateway.<realm>...") — the same segment
// internal/app/route_rules.go's routeTargetKind keys off, duplicated here
// since registry can't import internal/app (import direction is the other
// way).
func routeRuleTargetsInternetGateway(ocid string) bool {
	parts := strings.SplitN(ocid, ".", 3)
	return len(parts) >= 2 && parts[1] == "internetgateway"
}

// RouteTableRow wraps core.RouteTable with VcnName — the VCN's resolved
// display name (see (*RouteTableResource).List), so the VCN column can show
// a name instead of the raw VcnId, same "name over OCID" preference as
// drg_attachment.go's ATTACHED TO column.
type RouteTableRow struct {
	core.RouteTable
	VcnName string
}

func (r *RouteTableResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(RouteTableRow).DisplayName)
		}},
		{Header: "VCN", Width: 30, Get: func(row Row) string {
			return row.Raw.(RouteTableRow).VcnName
		}},
		{Header: "TYPE", Width: 8, Get: func(row Row) string {
			return routeTableAccessLabel(row.Raw.(RouteTableRow).RouteTable)
		}},
		{Header: "RULES", Width: 8, Get: func(row Row) string {
			return itoa(len(row.Raw.(RouteTableRow).RouteRules))
		}},
	}
}

func (r *RouteTableResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListRouteTablesRequest{CompartmentId: &s.CompartmentID}
	if s.VcnID != "" {
		req.VcnId = &s.VcnID
	}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListRouteTables(ctx, req)
	if err != nil {
		return nil, "", err
	}

	// Resolve each distinct VcnId to a name once (not once per row) — when
	// the table's already scoped to one VCN (the common case, browsing via
	// "i" on a VCN row), every route table shares the same VcnId, so a
	// per-row GetVcn would repeat the identical call N times.
	uniqueIDs := make([]string, 0, len(resp.Items))
	seen := map[string]bool{}
	for _, rt := range resp.Items {
		if id := deref(rt.VcnId); id != "" && !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	names := make([]string, len(uniqueIDs))
	var wg sync.WaitGroup
	for i, id := range uniqueIDs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			names[i] = vcnDisplayName(ctx, client, &id)
		}(i, id)
	}
	wg.Wait()
	vcnNameByID := make(map[string]string, len(uniqueIDs))
	for i, id := range uniqueIDs {
		vcnNameByID[id] = names[i]
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, rt := range resp.Items {
		rows = append(rows, Row{ID: deref(rt.Id), Name: deref(rt.DisplayName), TimeCreated: timeOf(rt.TimeCreated), Raw: RouteTableRow{
			RouteTable: rt,
			VcnName:    vcnNameByID[deref(rt.VcnId)],
		}})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
