package registry

import (
	"context"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
)

// fetchDbNodeStates returns the display-formatted lifecycle state of every
// DB node under the given DB system or Exadata VM cluster (exactly one of
// dbSystemID/vmClusterID should be non-nil). ListDbNodes requires either
// DbSystemId or VmClusterId — the SDK struct marks both individually
// "mandatory:false", which doesn't capture that "at least one of" rule;
// confirmed live, the API 400s ("MissingParameter") without one. So this
// is one call per row, not one per compartment — still cheap, since both
// DB systems and Exadata VM clusters are few per compartment. Most VM DB
// systems have exactly one node, but a 2-node RAC DB system or a
// multi-node Exadata VM cluster can have several — all are returned
// (joined by "/" in the NODE column, colored independently by
// colorizeState), since the parent resource can show "Available" while one
// of its nodes is independently stopped.
func fetchDbNodeStates(ctx context.Context, client database.DatabaseClient, compartmentID string, dbSystemID, vmClusterID *string) []string {
	nodes := fetchDbNodes(ctx, client, compartmentID, dbSystemID, vmClusterID)
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = stateLabel(n.LifecycleState)
	}
	return out
}

// fetchDbNodes is fetchDbNodeStates' full-summary counterpart, for callers
// (the Exascale node tree) that need more than just the lifecycle state —
// hostname, fault domain, CPU/memory — per node.
func fetchDbNodes(ctx context.Context, client database.DatabaseClient, compartmentID string, dbSystemID, vmClusterID *string) []database.DbNodeSummary {
	var out []database.DbNodeSummary
	page := ""
	for {
		req := database.ListDbNodesRequest{CompartmentId: &compartmentID, DbSystemId: dbSystemID, VmClusterId: vmClusterID}
		if page != "" {
			req.Page = &page
		}
		resp, err := client.ListDbNodes(ctx, req)
		if err != nil {
			return out
		}
		out = append(out, resp.Items...)
		if resp.OpcNextPage == nil {
			return out
		}
		page = *resp.OpcNextPage
	}
}

// fetchDbNodeIPs resolves each node's own host IP via HostIpId — the
// per-node connection address, as opposed to a cluster's shared SCAN IPs
// (see fetchPrivateIPs, used for both). DbNodeSummary carries only the
// PrivateIp OCID, not the address itself.
func fetchDbNodeIPs(ctx context.Context, vnClient core.VirtualNetworkClient, nodes []database.DbNodeSummary) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = deref(n.HostIpId)
	}
	return fetchPrivateIPs(ctx, vnClient, ids)
}

// fetchPrivateIPs resolves a list of PrivateIp OCIDs (a DB node's HostIpId,
// or an Exadata VM cluster's ScanIpIds) to their actual IPv4 addresses via
// GetPrivateIp — one call per id, fanned out concurrently since these lists
// stay short (few nodes/SCAN IPs per cluster), same reasoning as
// fetchInstanceIPs' per-instance GetVnic calls. A blank id or a failed call
// just leaves that slot empty. Result is parallel to ids (same index), not
// compacted, so positional callers (e.g. fetchDbNodeIPs, one entry per
// node) get that alignment for free.
func fetchPrivateIPs(ctx context.Context, vnClient core.VirtualNetworkClient, ids []string) []string {
	out := make([]string, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		if id == "" {
			continue
		}
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			resp, err := vnClient.GetPrivateIp(ctx, core.GetPrivateIpRequest{PrivateIpId: &id})
			if err != nil {
				return
			}
			out[i] = deref(resp.IpAddress)
		}(i, id)
	}
	wg.Wait()
	return out
}
