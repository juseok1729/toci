package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"toci/internal/clients"
)

type CompartmentResource struct {
	factory *clients.Factory
}

func NewCompartmentResource(f *clients.Factory) *CompartmentResource {
	return &CompartmentResource{factory: f}
}

func (r *CompartmentResource) Key() string   { return "compartment" }
func (r *CompartmentResource) Label() string { return "Compartments" }

func (r *CompartmentResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(identity.Compartment).Name)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(identity.Compartment).LifecycleState)
		}},
		{Header: "DESCRIPTION", Width: 40, Get: func(row Row) string {
			return deref(row.Raw.(identity.Compartment).Description)
		}},
	}
}

func (r *CompartmentResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.Identity(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := identity.ListCompartmentsRequest{
		CompartmentId:          &s.CompartmentID,
		LifecycleState:         identity.CompartmentLifecycleStateActive,
		CompartmentIdInSubtree: common.Bool(false),
	}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListCompartments(ctx, req)
	if err != nil {
		return nil, "", err
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, c := range resp.Items {
		rows = append(rows, Row{ID: deref(c.Id), Name: deref(c.Name), TimeCreated: timeOf(c.TimeCreated), Raw: c})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
