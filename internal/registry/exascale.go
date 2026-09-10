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
// as CloudVmClusterRow. Nodes carries the full per-node summary (hostname,
// fault domain, CPU/memory) for the node tree the app's "g" key expands
// under each cluster row; NodeStates stays a plain projection of it for the
// NODE column.
type ExadbVmClusterRow struct {
	database.ExadbVmClusterSummary
	NodeStates []string
	Nodes      []database.DbNodeSummary
	// NodeIPs is each node's own host IP (fetchDbNodeIPs, via HostIpId),
	// parallel to Nodes — shown per node in the "g" node tree.
	NodeIPs []string
	// ScanIPs is the cluster's shared SCAN listener IPs (resolved from
	// ExadbVmClusterSummary.ScanIpIds) — shown in the IP column before the
	// node tree is expanded, since a client connects to these, not to any
	// one node's address.
	ScanIPs []string
	// DiskTotalGB/DiskAvailGB are the shared Exadata Database Storage
	// Vault's capacity (fetchVaultStorage) — Exascale storage lives in the
	// vault (ExascaleDbStorageVaultId), not on the cluster itself, so this
	// is one extra GetExascaleDbStorageVault call per row. Zero if the
	// fetch failed, which the DISK% column's Get treats as "-".
	DiskTotalGB int
	DiskAvailGB int
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
		// Cluster-wide total (like ECPU above), not per node — divide by
		// NODES for the per-node figure, or check OCPU/the node tree below.
		{Header: "MEM(GB)", Width: 8, Get: func(row Row) string {
			mem := row.Raw.(ExadbVmClusterRow).MemorySizeInGBs
			if mem == nil {
				return "-"
			}
			return itoa(*mem)
		}},
		// Per-node OCPU count — distinct from the cluster-wide ECPU above
		// (ECPU/OCPU aren't 1:1). No single cluster-level OCPU figure exists,
		// so this is blank until "g" expands the node tree.
		{Header: "OCPU", Width: 6, Get: func(row Row) string {
			return "-"
		}},
		// Percent used of the shared storage vault's capacity — not the
		// cluster's own file system size (VmFileSystemStorage), since
		// Exascale storage is pooled in a vault other clusters can share.
		{Header: "DISK%", Width: 6, Get: func(row Row) string {
			d := row.Raw.(ExadbVmClusterRow)
			if d.DiskTotalGB == 0 {
				return "-"
			}
			used := d.DiskTotalGB - d.DiskAvailGB
			return itoa(used*100/d.DiskTotalGB) + "%"
		}},
		// Same width reasoning as CloudVmClusterResource's NODE column.
		{Header: "NODE", Width: 103, Get: func(row Row) string {
			states := row.Raw.(ExadbVmClusterRow).NodeStates
			if len(states) == 0 {
				return "-"
			}
			return strings.Join(states, "/")
		}},
		// SCAN IPs, not node IPs — a client connects to the cluster's SCAN
		// listener, not to any one node. Same width reasoning as NODE above,
		// though a SCAN list rarely exceeds 3 entries.
		{Header: "IP", Width: 127, Get: func(row Row) string {
			ips := row.Raw.(ExadbVmClusterRow).ScanIPs
			if len(ips) == 0 {
				return "-"
			}
			return strings.Join(ips, "/")
		}},
	}
}

// fetchVaultStorage resolves an Exadata Database Storage Vault's shared
// capacity (total/available GB) via GetExascaleDbStorageVault. Returns
// zeros on a nil id or a failed call, which the DISK% column treats as
// "unknown" rather than "0% used".
func fetchVaultStorage(ctx context.Context, client database.DatabaseClient, vaultID *string) (total, available int) {
	if vaultID == nil {
		return 0, 0
	}
	vault, err := client.GetExascaleDbStorageVault(ctx, database.GetExascaleDbStorageVaultRequest{ExascaleDbStorageVaultId: vaultID})
	if err != nil || vault.HighCapacityDatabaseStorage == nil {
		return 0, 0
	}
	s := vault.HighCapacityDatabaseStorage
	if s.TotalSizeInGbs != nil {
		total = *s.TotalSizeInGbs
	}
	if s.AvailableSizeInGbs != nil {
		available = *s.AvailableSizeInGbs
	}
	return total, available
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

	// Needed unconditionally (not just when VCN-filtered) to resolve SCAN
	// and node IPs below via GetPrivateIp.
	vnClient, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	var allow map[string]bool
	if s.VcnID != "" {
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
			nodes := fetchDbNodes(ctx, client, s.CompartmentID, nil, c.Id)
			states := make([]string, len(nodes))
			for j, n := range nodes {
				states[j] = stateLabel(n.LifecycleState)
			}
			ips := fetchDbNodeIPs(ctx, vnClient, nodes)
			scanIPs := fetchPrivateIPs(ctx, vnClient, c.ScanIpIds)
			diskTotal, diskAvail := fetchVaultStorage(ctx, client, c.ExascaleDbStorageVaultId)
			rows[i] = Row{ID: deref(c.Id), Name: deref(c.DisplayName), TimeCreated: timeOf(c.TimeCreated), Raw: ExadbVmClusterRow{
				ExadbVmClusterSummary: c,
				NodeStates:            states,
				Nodes:                 nodes,
				NodeIPs:               ips,
				ScanIPs:               scanIPs,
				DiskTotalGB:           diskTotal,
				DiskAvailGB:           diskAvail,
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
