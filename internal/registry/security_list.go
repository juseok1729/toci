package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

type SecurityListResource struct {
	factory *clients.Factory
}

func NewSecurityListResource(f *clients.Factory) *SecurityListResource {
	return &SecurityListResource{factory: f}
}

func (r *SecurityListResource) Key() string   { return "security-list" }
func (r *SecurityListResource) Label() string { return "Security Lists" }

func (r *SecurityListResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.SecurityList).DisplayName)
		}},
		{Header: "INGRESS", Width: 8, Get: func(row Row) string {
			return itoa(len(row.Raw.(core.SecurityList).IngressSecurityRules))
		}},
		{Header: "EGRESS", Width: 8, Get: func(row Row) string {
			return itoa(len(row.Raw.(core.SecurityList).EgressSecurityRules))
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return string(row.Raw.(core.SecurityList).LifecycleState)
		}},
	}
}

func (r *SecurityListResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListSecurityListsRequest{CompartmentId: &s.CompartmentID}
	if s.VcnID != "" {
		req.VcnId = &s.VcnID
	}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListSecurityLists(ctx, req)
	if err != nil {
		return nil, "", err
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, sl := range resp.Items {
		rows = append(rows, Row{ID: deref(sl.Id), Name: deref(sl.DisplayName), Raw: sl})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
