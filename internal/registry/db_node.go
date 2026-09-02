package registry

import (
	"context"

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
	var out []string
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
		for _, n := range resp.Items {
			out = append(out, stateLabel(n.LifecycleState))
		}
		if resp.OpcNextPage == nil {
			return out
		}
		page = *resp.OpcNextPage
	}
}
