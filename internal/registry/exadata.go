package registry

import (
	"context"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/database"
	"toci/internal/clients"
)

// CloudVmClusterResource is the Exadata Cloud Service VM cluster — the
// actual running database workload; the physical
// CloudExadataInfrastructure it sits on is a separate, compartment-level
// (not VCN-scoped) resource this app doesn't browse. CloudVmCluster carries
// a mandatory SubnetId, so it's always VCN-scoped.
type CloudVmClusterResource struct {
	factory *clients.Factory
}

func NewCloudVmClusterResource(f *clients.Factory) *CloudVmClusterResource {
	return &CloudVmClusterResource{factory: f}
}

func (r *CloudVmClusterResource) Key() string   { return "exadata" }
func (r *CloudVmClusterResource) Label() string { return "Exadata VM Clusters" }

func (r *CloudVmClusterResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(database.CloudVmClusterSummary).DisplayName)
		}},
		{Header: "STATE", Width: 14, Get: func(row Row) string {
			return stateLabel(row.Raw.(database.CloudVmClusterSummary).LifecycleState)
		}},
		{Header: "SHAPE", Width: 20, Get: func(row Row) string {
			return deref(row.Raw.(database.CloudVmClusterSummary).Shape)
		}},
		{Header: "NODES", Width: 6, Get: func(row Row) string {
			n := row.Raw.(database.CloudVmClusterSummary).NodeCount
			if n == nil {
				return "-"
			}
			return itoa(*n)
		}},
		{Header: "OCPU", Width: 6, Get: func(row Row) string {
			ocpu := row.Raw.(database.CloudVmClusterSummary).OcpuCount
			if ocpu == nil {
				return "-"
			}
			return fmt.Sprintf("%.1f", *ocpu)
		}},
	}
}

func (r *CloudVmClusterResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.Database(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := database.ListCloudVmClustersRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListCloudVmClusters(ctx, req)
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
	for _, c := range resp.Items {
		if allow != nil && !allow[deref(c.SubnetId)] {
			continue
		}
		rows = append(rows, Row{ID: deref(c.Id), Name: deref(c.DisplayName), TimeCreated: timeOf(c.TimeCreated), Raw: c})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
