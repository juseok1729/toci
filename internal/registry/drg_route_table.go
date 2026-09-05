package registry

import (
	"context"
	"errors"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

type DrgRouteTableResource struct {
	factory *clients.Factory
}

func NewDrgRouteTableResource(f *clients.Factory) *DrgRouteTableResource {
	return &DrgRouteTableResource{factory: f}
}

func (r *DrgRouteTableResource) Key() string   { return "drg-route-table" }
func (r *DrgRouteTableResource) Label() string { return "DRG Route Tables" }

func (r *DrgRouteTableResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.DrgRouteTable).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.DrgRouteTable).LifecycleState)
		}},
		{Header: "ECMP", Width: 6, Get: func(row Row) string {
			ecmp := row.Raw.(core.DrgRouteTable).IsEcmpEnabled
			if ecmp != nil && *ecmp {
				return "Yes"
			}
			return "No"
		}},
	}
}

// List requires s.DrgID — a DRG route table is always listed by its DRG
// (see ListDrgRouteTablesRequest.DrgId, mandatory), so this resource is
// only reachable after filtering by a DRG (see model.selectDrgFilter).
func (r *DrgRouteTableResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	if s.DrgID == "" {
		return nil, "", errors.New("select a DRG first (\"i\" or Enter on a DRG row)")
	}
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListDrgRouteTablesRequest{DrgId: &s.DrgID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListDrgRouteTables(ctx, req)
	if err != nil {
		return nil, "", err
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, t := range resp.Items {
		rows = append(rows, Row{ID: deref(t.Id), Name: deref(t.DisplayName), TimeCreated: timeOf(t.TimeCreated), Raw: t})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
