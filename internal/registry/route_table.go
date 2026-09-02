package registry

import (
	"context"

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

func (r *RouteTableResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.RouteTable).DisplayName)
		}},
		{Header: "RULES", Width: 8, Get: func(row Row) string {
			return itoa(len(row.Raw.(core.RouteTable).RouteRules))
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return string(row.Raw.(core.RouteTable).LifecycleState)
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

	rows := make([]Row, 0, len(resp.Items))
	for _, rt := range resp.Items {
		rows = append(rows, Row{ID: deref(rt.Id), Name: deref(rt.DisplayName), TimeCreated: timeOf(rt.TimeCreated), Raw: rt})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
