package registry

import (
	"context"
	"errors"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

type DrgRouteDistributionResource struct {
	factory *clients.Factory
}

func NewDrgRouteDistributionResource(f *clients.Factory) *DrgRouteDistributionResource {
	return &DrgRouteDistributionResource{factory: f}
}

func (r *DrgRouteDistributionResource) Key() string   { return "drg-route-distribution" }
func (r *DrgRouteDistributionResource) Label() string { return "DRG Route Distributions" }

func (r *DrgRouteDistributionResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.DrgRouteDistribution).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.DrgRouteDistribution).LifecycleState)
		}},
		{Header: "TYPE", Width: 8, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.DrgRouteDistribution).DistributionType)
		}},
	}
}

// List requires s.DrgID — a DRG route distribution is always listed by its
// DRG (see ListDrgRouteDistributionsRequest.DrgId, mandatory), so this
// resource is only reachable after filtering by a DRG (see
// model.selectDrgFilter).
func (r *DrgRouteDistributionResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	if s.DrgID == "" {
		return nil, "", errors.New("select a DRG first (\"i\" or Enter on a DRG row)")
	}
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListDrgRouteDistributionsRequest{DrgId: &s.DrgID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListDrgRouteDistributions(ctx, req)
	if err != nil {
		return nil, "", err
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, d := range resp.Items {
		rows = append(rows, Row{ID: deref(d.Id), Name: deref(d.DisplayName), TimeCreated: timeOf(d.TimeCreated), Raw: d})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
