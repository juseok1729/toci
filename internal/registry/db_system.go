package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/database"
	"toci/internal/clients"
)

// DbSystemResource is the Base Database service (VM/Bare Metal DB systems).
// Every DbSystem carries a mandatory SubnetId, so it's always VCN-scoped.
type DbSystemResource struct {
	factory *clients.Factory
}

func NewDbSystemResource(f *clients.Factory) *DbSystemResource {
	return &DbSystemResource{factory: f}
}

func (r *DbSystemResource) Key() string   { return "db-system" }
func (r *DbSystemResource) Label() string { return "DB Systems" }

func (r *DbSystemResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(database.DbSystemSummary).DisplayName)
		}},
		{Header: "STATE", Width: 14, Get: func(row Row) string {
			return stateLabel(row.Raw.(database.DbSystemSummary).LifecycleState)
		}},
		{Header: "SHAPE", Width: 24, Get: func(row Row) string {
			return deref(row.Raw.(database.DbSystemSummary).Shape)
		}},
		{Header: "EDITION", Width: 39, Get: func(row Row) string {
			return string(row.Raw.(database.DbSystemSummary).DatabaseEdition)
		}},
		{Header: "OCPU", Width: 6, Get: func(row Row) string {
			cpu := row.Raw.(database.DbSystemSummary).CpuCoreCount
			if cpu == nil {
				return "-"
			}
			return itoa(*cpu)
		}},
	}
}

func (r *DbSystemResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.Database(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := database.ListDbSystemsRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListDbSystems(ctx, req)
	if err != nil {
		return nil, "", err
	}

	var allow map[string]bool
	if s.VcnID != "" {
		vnClient, err := r.factory.VirtualNetwork(s.Region)
		if err != nil {
			return nil, "", err
		}
		allow, err = vcnSubnetIDs(ctx, vnClient, s.CompartmentID, s.VcnID)
		if err != nil {
			return nil, "", err
		}
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, d := range resp.Items {
		if allow != nil && !allow[deref(d.SubnetId)] {
			continue
		}
		rows = append(rows, Row{ID: deref(d.Id), Name: deref(d.DisplayName), TimeCreated: timeOf(d.TimeCreated), Raw: d})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
