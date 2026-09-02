package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

type DrgResource struct {
	factory *clients.Factory
}

func NewDrgResource(f *clients.Factory) *DrgResource {
	return &DrgResource{factory: f}
}

func (r *DrgResource) Key() string   { return "drg" }
func (r *DrgResource) Label() string { return "DRGs" }

func (r *DrgResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.Drg).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.Drg).LifecycleState)
		}},
	}
}

func (r *DrgResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListDrgsRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListDrgs(ctx, req)
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
