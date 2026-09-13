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

// subnetAccessLabel reports whether a subnet is Public or Private —
// ProhibitPublicIpOnVnic is the field OCI's own console bases that label
// on: true means no VNIC in the subnet may get a public IP.
func subnetAccessLabel(sn core.Subnet) string {
	if sn.ProhibitPublicIpOnVnic != nil && *sn.ProhibitPublicIpOnVnic {
		return "Private"
	}
	return "Public"
}

func (r *SubnetResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.Subnet).DisplayName)
		}},
		{Header: "TYPE", Width: 8, Get: func(row Row) string {
			return subnetAccessLabel(row.Raw.(core.Subnet))
		}},
		{Header: "CIDR", Width: 18, Get: func(row Row) string {
			return deref(row.Raw.(core.Subnet).CidrBlock)
		}},
		{Header: "IP RANGE", Width: 46, Get: func(row Row) string {
			return cidrRange(deref(row.Raw.(core.Subnet).CidrBlock))
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
		rows = append(rows, Row{ID: deref(sn.Id), Name: deref(sn.DisplayName), TimeCreated: timeOf(sn.TimeCreated), Raw: sn})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
