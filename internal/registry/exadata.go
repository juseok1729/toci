package registry

import (
	"context"
	"fmt"
	"strings"
	"sync"

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

// CloudVmClusterRow adds each DB node's own lifecycle state alongside the
// SDK summary — same reasoning as DbSystemRow: the cluster's own STATE can
// say "Available" while one of its nodes was independently stopped.
type CloudVmClusterRow struct {
	database.CloudVmClusterSummary
	NodeStates []string
}

func NewCloudVmClusterResource(f *clients.Factory) *CloudVmClusterResource {
	return &CloudVmClusterResource{factory: f}
}

func (r *CloudVmClusterResource) Key() string   { return "exadata" }
func (r *CloudVmClusterResource) Label() string { return "Exadata VM Clusters" }

func (r *CloudVmClusterResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(CloudVmClusterRow).DisplayName)
		}},
		{Header: "STATE", Width: 14, Get: func(row Row) string {
			return stateLabel(row.Raw.(CloudVmClusterRow).LifecycleState)
		}},
		{Header: "SHAPE", Width: 20, Get: func(row Row) string {
			return deref(row.Raw.(CloudVmClusterRow).Shape)
		}},
		{Header: "NODES", Width: 6, Get: func(row Row) string {
			n := row.Raw.(CloudVmClusterRow).NodeCount
			if n == nil {
				return "-"
			}
			return itoa(*n)
		}},
		{Header: "OCPU", Width: 6, Get: func(row Row) string {
			ocpu := row.Raw.(CloudVmClusterRow).OcpuCount
			if ocpu == nil {
				return "-"
			}
			return fmt.Sprintf("%.1f", *ocpu)
		}},
		// Unlike a VM DB system's 2-node RAC cap, an Exadata VM cluster can
		// span many more nodes — width generously covers up to 8 at the
		// longest node state word ("Terminating"/"Provisioning", 12 chars):
		// 8*12 + 7 separators = 103. Same ceiling-is-a-hard-cap risk noted
		// on DbSystemResource's NODE/ROLE columns.
		{Header: "NODE", Width: 103, Get: func(row Row) string {
			states := row.Raw.(CloudVmClusterRow).NodeStates
			if len(states) == 0 {
				return "-"
			}
			return strings.Join(states, "/")
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

	items := make([]database.CloudVmClusterSummary, 0, len(resp.Items))
	for _, c := range resp.Items {
		if allow == nil || allow[deref(c.SubnetId)] {
			items = append(items, c)
		}
	}

	// Same tradeoff as DbSystemResource.List: fetchDbNodeStates is one call
	// per row, so fan every row out to its own goroutine (each writes only
	// its own rows[i], no shared state to lock) instead of paying for it
	// one row at a time.
	rows := make([]Row, len(items))
	var wg sync.WaitGroup
	for i, c := range items {
		wg.Add(1)
		go func(i int, c database.CloudVmClusterSummary) {
			defer wg.Done()
			rows[i] = Row{ID: deref(c.Id), Name: deref(c.DisplayName), TimeCreated: timeOf(c.TimeCreated), Raw: CloudVmClusterRow{
				CloudVmClusterSummary: c,
				NodeStates:            fetchDbNodeStates(ctx, client, s.CompartmentID, nil, c.Id),
			}}
		}(i, c)
	}
	wg.Wait()

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
