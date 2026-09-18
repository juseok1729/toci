package registry

import (
	"context"
	"fmt"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/mysql"
	"toci/internal/clients"
)

// MysqlDbSystemResource is the MySQL HeatWave service (formerly MySQL
// Database Service): managed MySQL DB systems, optionally with a HeatWave
// analytics cluster attached. Not VCN-scoped: ListDbSystems' summary has
// no SubnetId (only GetDbSystem does), so it isn't in the app's
// vcnScopedResourceKeys set.
type MysqlDbSystemResource struct {
	factory *clients.Factory
}

// MysqlDbSystemRow adds the full HeatWave cluster (GetHeatWaveCluster,
// one extra call per row that has one attached) alongside the SDK
// summary — the summary only carries the cluster's shape/size/state, not
// its nodes, which the app's "g" node tree lists under the DB system row.
// nil when no cluster is attached (or the fetch failed).
type MysqlDbSystemRow struct {
	mysql.DbSystemSummary
	HeatWave *mysql.HeatWaveCluster `yaml:"heatWave,omitempty"`
}

func NewMysqlDbSystemResource(f *clients.Factory) *MysqlDbSystemResource {
	return &MysqlDbSystemResource{factory: f}
}

func (r *MysqlDbSystemResource) Key() string   { return "mysql" }
func (r *MysqlDbSystemResource) Label() string { return "MySQL HeatWave" }

// HeatWaveLabel is the HEATWAVE column's text: the attached cluster's
// state and node count ("Active (2)"), or "-" for a plain DB system —
// how the table tells a HeatWave cluster apart from a standalone MySQL.
func (row MysqlDbSystemRow) HeatWaveLabel() string {
	hw := row.HeatWaveCluster
	if hw == nil || row.IsHeatWaveClusterAttached == nil || !*row.IsHeatWaveClusterAttached {
		return "-"
	}
	size := 0
	if hw.ClusterSize != nil {
		size = *hw.ClusterSize
	}
	return fmt.Sprintf("%s (%d)", stateLabel(hw.LifecycleState), size)
}

// PrimaryEndpoint returns the DB system's own read/write endpoint (the
// DBSYSTEM one; read replicas and read endpoints are listed separately),
// falling back to the first endpoint of any kind. ok=false with none.
func (row MysqlDbSystemRow) PrimaryEndpoint() (ep mysql.DbSystemEndpoint, ok bool) {
	for _, e := range row.Endpoints {
		if e.ResourceType == mysql.DbSystemEndpointResourceTypeDbsystem {
			return e, true
		}
	}
	if len(row.Endpoints) > 0 {
		return row.Endpoints[0], true
	}
	return ep, false
}

func (r *MysqlDbSystemResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(MysqlDbSystemRow).DisplayName)
		}},
		{Header: "STATE", Width: 14, Get: func(row Row) string {
			return stateLabel(row.Raw.(MysqlDbSystemRow).LifecycleState)
		}},
		{Header: "VERSION", Width: 10, Get: func(row Row) string {
			return deref(row.Raw.(MysqlDbSystemRow).MysqlVersion)
		}},
		{Header: "SHAPE", Width: 28, Get: func(row Row) string {
			return deref(row.Raw.(MysqlDbSystemRow).ShapeName)
		}},
		// HA = the 3-instance highly-available DB system option; separate
		// from HeatWave (an analytics cluster attached to the DB system).
		{Header: "HA", Width: 4, Get: func(row Row) string {
			if ha := row.Raw.(MysqlDbSystemRow).IsHighlyAvailable; ha != nil && *ha {
				return "yes"
			}
			return "-"
		}},
		{Header: "HEATWAVE", Width: 16, Get: func(row Row) string {
			return row.Raw.(MysqlDbSystemRow).HeatWaveLabel()
		}},
		// ip:port of the read/write endpoint — the whole connection
		// string is a keypress away (Enter), this is the at-a-glance bit.
		{Header: "ENDPOINT", Width: 21, Get: func(row Row) string {
			ep, ok := row.Raw.(MysqlDbSystemRow).PrimaryEndpoint()
			if !ok || ep.IpAddress == nil {
				return "-"
			}
			if ep.Port == nil {
				return *ep.IpAddress
			}
			return fmt.Sprintf("%s:%d", *ep.IpAddress, *ep.Port)
		}},
	}
}

func (r *MysqlDbSystemResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.Mysql(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := mysql.ListDbSystemsRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}
	resp, err := client.ListDbSystems(ctx, req)
	if err != nil {
		return nil, "", err
	}

	// One GetHeatWaveCluster per attached cluster, fanned out like
	// DbSystemResource's per-row enrichment — DB systems per compartment
	// are few, and the rows without a cluster cost nothing extra.
	rows := make([]Row, len(resp.Items))
	var wg sync.WaitGroup
	for i, d := range resp.Items {
		wg.Add(1)
		go func(i int, d mysql.DbSystemSummary) {
			defer wg.Done()
			row := MysqlDbSystemRow{DbSystemSummary: d}
			if d.IsHeatWaveClusterAttached != nil && *d.IsHeatWaveClusterAttached {
				if hw, err := client.GetHeatWaveCluster(ctx, mysql.GetHeatWaveClusterRequest{DbSystemId: d.Id}); err == nil {
					row.HeatWave = &hw.HeatWaveCluster
				}
			}
			rows[i] = Row{ID: deref(d.Id), Name: deref(d.DisplayName), TimeCreated: timeOf(d.TimeCreated), Raw: row}
		}(i, d)
	}
	wg.Wait()

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
