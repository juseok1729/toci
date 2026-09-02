package registry

import (
	"context"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/database"
	"toci/internal/clients"
)

// AutonomousDatabaseResource is the Autonomous Database service. SubnetId is
// optional here — only set when the ADB uses a private endpoint (e.g. on
// dedicated Exadata infrastructure); a "public" serverless ADB has none and
// simply won't match any VCN filter.
type AutonomousDatabaseResource struct {
	factory *clients.Factory
}

func NewAutonomousDatabaseResource(f *clients.Factory) *AutonomousDatabaseResource {
	return &AutonomousDatabaseResource{factory: f}
}

func (r *AutonomousDatabaseResource) Key() string   { return "adb" }
func (r *AutonomousDatabaseResource) Label() string { return "Autonomous DBs" }

func adbName(a database.AutonomousDatabaseSummary) string {
	if a.DisplayName != nil {
		return *a.DisplayName
	}
	return deref(a.DbName)
}

func (r *AutonomousDatabaseResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return adbName(row.Raw.(database.AutonomousDatabaseSummary))
		}},
		{Header: "WORKLOAD", Width: 14, Get: func(row Row) string {
			return string(row.Raw.(database.AutonomousDatabaseSummary).DbWorkload)
		}},
		{Header: "OCPU", Width: 6, Get: func(row Row) string {
			a := row.Raw.(database.AutonomousDatabaseSummary)
			switch {
			case a.ComputeCount != nil:
				return fmt.Sprintf("%.1f", *a.ComputeCount)
			case a.CpuCoreCount != nil:
				return itoa(*a.CpuCoreCount)
			default:
				return "-"
			}
		}},
		{Header: "STORAGE(TB)", Width: 12, Get: func(row Row) string {
			tb := row.Raw.(database.AutonomousDatabaseSummary).DataStorageSizeInTBs
			if tb == nil {
				return "-"
			}
			return itoa(*tb)
		}},
		{Header: "STATE", Width: 14, Get: func(row Row) string {
			return string(row.Raw.(database.AutonomousDatabaseSummary).LifecycleState)
		}},
	}
}

func (r *AutonomousDatabaseResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.Database(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := database.ListAutonomousDatabasesRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListAutonomousDatabases(ctx, req)
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
	for _, a := range resp.Items {
		if allow != nil && !allow[deref(a.SubnetId)] {
			continue
		}
		rows = append(rows, Row{ID: deref(a.Id), Name: adbName(a), TimeCreated: timeOf(a.TimeCreated), Raw: a})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
