package registry

import (
	"context"
	"strings"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/database"
	"toci/internal/clients"
)

// ExadbVmClusterResource is the Exadata VM cluster on Exascale
// Infrastructure — the newer Exadata Database Service that uses ECPUs and a
// shared Exascale storage vault instead of the classic Exadata Cloud
// Service's dedicated storage cells. Like CloudVmClusterResource, it carries
// a mandatory SubnetId, so it's always VCN-scoped.
type ExadbVmClusterResource struct {
	factory *clients.Factory
}

// ExadbVmClusterRow adds each DB node's own lifecycle state — same reasoning
// as CloudVmClusterRow.
type ExadbVmClusterRow struct {
	database.ExadbVmClusterSummary
	NodeStates []string
}

func NewExadbVmClusterResource(f *clients.Factory) *ExadbVmClusterResource {
	return &ExadbVmClusterResource{factory: f}
}

func (r *ExadbVmClusterResource) Key() string   { return "exascale" }
func (r *ExadbVmClusterResource) Label() string { return "Exadata VM Clusters (Exascale)" }

// licenseModelAbbrev mirrors dbEditionAbbrev's reasoning: the raw enum
// ("BRING_YOUR_OWN_LICENSE") is too wide for a column.
func licenseModelAbbrev(m database.ExadbVmClusterSummaryLicenseModelEnum) string {
	switch m {
	case database.ExadbVmClusterSummaryLicenseModelBringYourOwnLicense:
		return "BYOL"
	case database.ExadbVmClusterSummaryLicenseModelLicenseIncluded:
		return "Included"
	default:
		return string(m)
	}
}

func (r *ExadbVmClusterResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(ExadbVmClusterRow).DisplayName)
		}},
		{Header: "STATE", Width: 14, Get: func(row Row) string {
			return stateLabel(row.Raw.(ExadbVmClusterRow).LifecycleState)
		}},
		{Header: "SHAPE", Width: 20, Get: func(row Row) string {
			return deref(row.Raw.(ExadbVmClusterRow).Shape)
		}},
		{Header: "LICENSE", Width: 8, Get: func(row Row) string {
			return licenseModelAbbrev(row.Raw.(ExadbVmClusterRow).LicenseModel)
		}},
		{Header: "NODES", Width: 6, Get: func(row Row) string {
			n := row.Raw.(ExadbVmClusterRow).NodeCount
			if n == nil {
				return "-"
			}
			return itoa(*n)
		}},
		{Header: "ECPU", Width: 6, Get: func(row Row) string {
			ecpu := row.Raw.(ExadbVmClusterRow).EnabledECpuCount
			if ecpu == nil {
				return "-"
			}
			return itoa(*ecpu)
		}},
		// Same width reasoning as CloudVmClusterResource's NODE column.
		{Header: "NODE", Width: 103, Get: func(row Row) string {
			states := row.Raw.(ExadbVmClusterRow).NodeStates
			if len(states) == 0 {
				return "-"
			}
			return strings.Join(states, "/")
		}},
	}
}

func (r *ExadbVmClusterResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.Database(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := database.ListExadbVmClustersRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListExadbVmClusters(ctx, req)
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

	items := make([]database.ExadbVmClusterSummary, 0, len(resp.Items))
	for _, c := range resp.Items {
		if allow == nil || allow[deref(c.SubnetId)] {
			items = append(items, c)
		}
	}

	rows := make([]Row, len(items))
	var wg sync.WaitGroup
	for i, c := range items {
		wg.Add(1)
		go func(i int, c database.ExadbVmClusterSummary) {
			defer wg.Done()
			rows[i] = Row{ID: deref(c.Id), Name: deref(c.DisplayName), TimeCreated: timeOf(c.TimeCreated), Raw: ExadbVmClusterRow{
				ExadbVmClusterSummary: c,
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
