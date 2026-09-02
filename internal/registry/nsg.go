package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

type NsgResource struct {
	factory *clients.Factory
}

func NewNsgResource(f *clients.Factory) *NsgResource {
	return &NsgResource{factory: f}
}

func (r *NsgResource) Key() string   { return "nsg" }
func (r *NsgResource) Label() string { return "NSG" }

func (r *NsgResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.NetworkSecurityGroup).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.NetworkSecurityGroup).LifecycleState)
		}},
	}
}

func (r *NsgResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListNetworkSecurityGroupsRequest{CompartmentId: &s.CompartmentID}
	if s.VcnID != "" {
		req.VcnId = &s.VcnID
	}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListNetworkSecurityGroups(ctx, req)
	if err != nil {
		return nil, "", err
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, n := range resp.Items {
		rows = append(rows, Row{ID: deref(n.Id), Name: deref(n.DisplayName), TimeCreated: timeOf(n.TimeCreated), Raw: n})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
