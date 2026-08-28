package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

type SubnetResource struct {
	factory *clients.Factory
}

func NewSubnetResource(f *clients.Factory) *SubnetResource {
	return &SubnetResource{factory: f}
}

func (r *SubnetResource) Key() string   { return "subnet" }
func (r *SubnetResource) Label() string { return "Subnets" }

func (r *SubnetResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.Subnet).DisplayName)
		}},
		{Header: "CIDR", Width: 18, Get: func(row Row) string {
			return deref(row.Raw.(core.Subnet).CidrBlock)
		}},
		{Header: "AD", Width: 20, Get: func(row Row) string {
			return deref(row.Raw.(core.Subnet).AvailabilityDomain)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return string(row.Raw.(core.Subnet).LifecycleState)
		}},
	}
}

func (r *SubnetResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListSubnetsRequest{CompartmentId: &s.CompartmentID}
	if s.VcnID != "" {
		req.VcnId = &s.VcnID
	}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListSubnets(ctx, req)
	if err != nil {
		return nil, "", err
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, sn := range resp.Items {
		rows = append(rows, Row{ID: deref(sn.Id), Name: deref(sn.DisplayName), Raw: sn})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
