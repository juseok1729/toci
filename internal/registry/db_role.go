package registry

import (
	"context"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/database"
)

// fetchDbSystemRole returns the Data Guard role ("Primary", "Standby", ...)
// of the first database under the given DB system, or "" if it has none —
// either a standalone database with no Data Guard association, or the
// association lookup failed. If the peer database lives in a different
// region than the given one, that region is appended (e.g. "Primary→tokyo")
// — Data Guard associations can be cross-region, and the peer DB system
// simply doesn't appear at all in this region's own DB Systems list, so
// this is the only hint that it exists somewhere else. The peer's region
// comes for free from its OCID (see regionFromOCID) — no extra API call.
//
// ListDatabases(SystemId) is DB-system-scoped (cheap, like GetDbSystem
// elsewhere in this file), but the actual role lives on the database, not
// the DB system — reached only via a further per-database
// ListDataGuardAssociations call. So this costs up to 1+N calls (N =
// databases under the DB system, almost always 1), acceptable since DB
// systems per compartment are few. The SDK's ListDatabasesRequest.SystemId
// field is documented as "Applies only to Exadata DB systems", but that's
// stale — confirmed live against a real VM.Standard DB system, which it
// filters correctly.
func fetchDbSystemRole(ctx context.Context, client database.DatabaseClient, compartmentID, dbSystemID, region string) string {
	resp, err := client.ListDatabases(ctx, database.ListDatabasesRequest{
		CompartmentId: &compartmentID,
		SystemId:      &dbSystemID,
	})
	if err != nil || len(resp.Items) == 0 || resp.Items[0].Id == nil {
		return ""
	}
	assoc, err := client.ListDataGuardAssociations(ctx, database.ListDataGuardAssociationsRequest{
		DatabaseId: resp.Items[0].Id,
	})
	if err != nil || len(assoc.Items) == 0 {
		return ""
	}
	a := assoc.Items[0]
	role := stateLabel(a.Role)
	if peerRegion := regionFromOCID(deref(a.PeerDbSystemId)); peerRegion != "" && peerRegion != region {
		role += "→" + peerRegion
	}
	return role
}

// regionFromOCID extracts the region segment from a regional OCID (e.g.
// "ocid1.dbsystem.oc1.ap-seoul-1.<unique>" -> "ap-seoul-1"). Returns "" for
// a tenancy-scoped OCID with no region segment (e.g.
// "ocid1.tenancy.oc1..<unique>", which splits with an empty 4th part) or
// any string too short to be an OCID.
func regionFromOCID(ocid string) string {
	parts := strings.Split(ocid, ".")
	if len(parts) < 5 {
		return ""
	}
	return parts[3]
}

// dbSystemRoleLabel decides what the ROLE column shows. A multi-node DB
// system is RAC by construction (see docs/COLOR_SYSTEM.md's earlier
// research) — that wins over any Data Guard role, since a RAC database can
// still participate in Data Guard, but its RAC-ness is the more
// fundamental architectural fact for a column this narrow.
func dbSystemRoleLabel(nodeStates []string, role string) string {
	if len(nodeStates) > 1 {
		return "RAC"
	}
	if role == "" {
		return "-"
	}
	return role
}
