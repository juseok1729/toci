package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

type VcnResource struct {
	factory *clients.Factory
}

func NewVcnResource(f *clients.Factory) *VcnResource {
	return &VcnResource{factory: f}
}

func (r *VcnResource) Key() string   { return "vcn" }
func (r *VcnResource) Label() string { return "VCNs" }

func (r *VcnResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.Vcn).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.Vcn).LifecycleState)
		}},
		{Header: "CIDR", Width: 18, Get: func(row Row) string {
			return deref(row.Raw.(core.Vcn).CidrBlock)
		}},
	}
}

func (r *VcnResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListVcnsRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListVcns(ctx, req)
	if err != nil {
		return nil, "", err
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, v := range resp.Items {
		rows = append(rows, Row{ID: deref(v.Id), Name: deref(v.DisplayName), TimeCreated: timeOf(v.TimeCreated), Raw: v})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
