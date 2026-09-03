package registry

import (
	"context"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/database"
	"toci/internal/clients"
)

// DbSystemResource is the Base Database service (VM/Bare Metal DB systems).
// Every DbSystem carries a mandatory SubnetId, so it's always VCN-scoped.
type DbSystemResource struct {
	factory *clients.Factory
}

// DbSystemRow adds each DB node's own lifecycle state alongside the SDK
// summary — the DbSystem's own STATE (e.g. "Available") only reflects
// whether the DbSystem resource itself is manageable, not whether the node
// running the database was separately stopped. See NODE column below.
type DbSystemRow struct {
	database.DbSystemSummary
	NodeStates []string
	Role       string // Data Guard role ("Primary"/"Standby"/...), "" if none
}

func NewDbSystemResource(f *clients.Factory) *DbSystemResource {
	return &DbSystemResource{factory: f}
}

// dbEditionAbbrev maps OCI's DatabaseEdition enum to the abbreviation
// Oracle's own licensing docs and the industry actually use — the full
// enum name ("ENTERPRISE_EDITION_EXTREME_PERFORMANCE") is far too wide for
// a column. Falls back to the raw enum string for any value added to the
// SDK after this mapping was written.
func dbEditionAbbrev(edition database.DbSystemSummaryDatabaseEditionEnum) string {
	switch edition {
	case database.DbSystemSummaryDatabaseEditionStandardEdition:
		return "SE2"
	case database.DbSystemSummaryDatabaseEditionEnterpriseEdition:
		return "EE"
	case database.DbSystemSummaryDatabaseEditionEnterpriseEditionHighPerformance:
		return "EE-HP"
	case database.DbSystemSummaryDatabaseEditionEnterpriseEditionExtremePerformance:
		return "EE-EP"
	case database.DbSystemSummaryDatabaseEditionEnterpriseEditionDeveloper:
		return "EE-DEV"
	default:
		return string(edition)
	}
}

func (r *DbSystemResource) Key() string   { return "db-system" }
func (r *DbSystemResource) Label() string { return "DB Systems" }

func (r *DbSystemResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(DbSystemRow).DisplayName)
		}},
		{Header: "STATE", Width: 14, Get: func(row Row) string {
			return stateLabel(row.Raw.(DbSystemRow).LifecycleState)
		}},
		{Header: "SHAPE", Width: 24, Get: func(row Row) string {
			return deref(row.Raw.(DbSystemRow).Shape)
		}},
		{Header: "EDITION", Width: 6, Get: func(row Row) string {
			return dbEditionAbbrev(row.Raw.(DbSystemRow).DatabaseEdition)
		}},
		// Straight off ListDbSystems, like SHAPE/EDITION — no extra call.
		{Header: "VERSION", Width: 12, Get: func(row Row) string {
			v := row.Raw.(DbSystemRow).Version
			if v == nil {
				return "-"
			}
			return *v
		}},
		{Header: "OCPU", Width: 6, Get: func(row Row) string {
			cpu := row.Raw.(DbSystemRow).CpuCoreCount
			if cpu == nil {
				return "-"
			}
			return itoa(*cpu)
		}},
		{Header: "MEM(GB)", Width: 8, Get: func(row Row) string {
			mem := row.Raw.(DbSystemRow).MemorySizeInGBs
			if mem == nil {
				return "-"
			}
			return itoa(*mem)
		}},
		// DATA + RECO — the two storage pools OCI allocates for a VM DB
		// system's ASM diskgroups (user data/database files, and
		// redo/archive logs/RMAN backups respectively) — matching the
		// Instance table's DISK(GB), which is also a total across every
		// attached volume. Both fields come straight off ListDbSystems, no
		// extra call needed (unlike MEM(GB) above).
		{Header: "DISK(GB)", Width: 8, Get: func(row Row) string {
			d := row.Raw.(DbSystemRow)
			if d.DataStorageSizeInGBs == nil && d.RecoStorageSizeInGB == nil {
				return "-"
			}
			total := 0
			if d.DataStorageSizeInGBs != nil {
				total += *d.DataStorageSizeInGBs
			}
			if d.RecoStorageSizeInGB != nil {
				total += *d.RecoStorageSizeInGB
			}
			return itoa(total)
		}},
		// Width covers 2 nodes (VM DB systems top out at 2-node RAC) of the
		// longest node state word, "Terminating"/"Provisioning" (12 chars):
		// 2*12 + 1 separator = 25. Same ceiling-is-a-hard-cap risk as ROLE
		// above.
		{Header: "NODE", Width: 25, Get: func(row Row) string {
			states := row.Raw.(DbSystemRow).NodeStates
			if len(states) == 0 {
				return "-"
			}
			return strings.Join(states, "/")
		}},
		// Width has to cover the worst case, not just "Primary"/"Standby"/
		// "RAC"/"-" (<=7 chars) — a cross-region Data Guard peer appends
		// "→<region>" (see fetchDbSystemRole), and fitColumnWidth's ceiling
		// is a hard cap, not just a hint: it truncates rather than growing
		// past a too-small declared Width. "Standby→af-johannesburg-1", the
		// longest real OCI region name, is 25 characters.
		{Header: "ROLE", Width: 25, Get: func(row Row) string {
			r := row.Raw.(DbSystemRow)
			return dbSystemRoleLabel(r.NodeStates, r.Role)
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
		// ListDbSystems never populates MemorySizeInGBs (confirmed against a
		// real DB system: List returns nil, Get returns the real value) —
		// only GetDbSystem does. DB systems per compartment are few (unlike
		// instances), so one Get call per row here is cheap, the same
		// tradeoff instance_ip.go already accepts for GetVnic.
		if d.MemorySizeInGBs == nil {
			if full, err := client.GetDbSystem(ctx, database.GetDbSystemRequest{DbSystemId: d.Id}); err == nil {
				d.MemorySizeInGBs = full.MemorySizeInGBs
			}
		}
		nodeStates := fetchDbNodeStates(ctx, client, s.CompartmentID, d.Id, nil)
		var role string
		if len(nodeStates) <= 1 {
			// RAC (len > 1) wins in the ROLE column regardless, so skip the
			// extra ListDatabases+ListDataGuardAssociations calls for it.
			role = fetchDbSystemRole(ctx, client, s.CompartmentID, deref(d.Id), s.Region)
		}
		rows = append(rows, Row{ID: deref(d.Id), Name: deref(d.DisplayName), TimeCreated: timeOf(d.TimeCreated), Raw: DbSystemRow{
			DbSystemSummary: d,
			NodeStates:      nodeStates,
			Role:            role,
		}})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
