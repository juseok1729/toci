package registry

import (
	"context"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/containerengine"
	"toci/internal/clients"
)

// NodePoolRow adds each node's own detail (Id/Name/LifecycleState/
// KubernetesVersion, fetched via GetNodePool — a ListNodePools summary
// alone carries no Nodes) for the app's "g" key to expand into a per-node
// tree under the pool row, same idea as ExadbVmClusterRow.Nodes.
type NodePoolRow struct {
	containerengine.NodePoolSummary
	Nodes []containerengine.Node
}

// NodePoolResource lists an OKE cluster's node pools — only reachable
// scoped to one cluster (Scope.OkeID), via Enter on an "oke" row's
// "Node Pools" menu item, the same way DrgRouteTableResource only makes
// sense scoped to a DRG.
type NodePoolResource struct {
	factory *clients.Factory
}

func NewNodePoolResource(f *clients.Factory) *NodePoolResource {
	return &NodePoolResource{factory: f}
}

func (r *NodePoolResource) Key() string   { return "oke-node-pool" }
func (r *NodePoolResource) Label() string { return "Node Pools" }

func (r *NodePoolResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(NodePoolRow).Name)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(NodePoolRow).LifecycleState)
		}},
		{Header: "K8S VERSION", Width: 14, Get: func(row Row) string {
			return deref(row.Raw.(NodePoolRow).KubernetesVersion)
		}},
		{Header: "SHAPE", Width: 20, Get: func(row Row) string {
			return deref(row.Raw.(NodePoolRow).NodeShape)
		}},
		// Ocpus/MemoryInGBs are per-node figures (NodeShapeConfig's own doc:
		// "available to each node in the node pool") — every node under
		// this pool gets exactly this shape, unlike Exascale's ECPU/MEM(GB)
		// columns, which are a cluster-wide total. nil for a fixed (non-
		// ".Flex") shape, whose OCPU/memory aren't independently configurable.
		{Header: "OCPU", Width: 6, Get: func(row Row) string {
			cfg := row.Raw.(NodePoolRow).NodeShapeConfig
			if cfg == nil {
				return "-"
			}
			return floatString(cfg.Ocpus)
		}},
		{Header: "MEM(GB)", Width: 8, Get: func(row Row) string {
			cfg := row.Raw.(NodePoolRow).NodeShapeConfig
			if cfg == nil {
				return "-"
			}
			return floatString(cfg.MemoryInGBs)
		}},
		{Header: "SIZE", Width: 6, Get: func(row Row) string {
			d := row.Raw.(NodePoolRow).NodeConfigDetails
			if d == nil || d.Size == nil {
				return "-"
			}
			return itoa(*d.Size)
		}},
	}
}

// List requires Scope.OkeID (see NodePoolResource's own doc) — the caller
// (openOkeResourceSearch/switchResource's okeIDRequiredResourceKeys
// redirect) is what guarantees it's set before this ever runs.
func (r *NodePoolResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.ContainerEngine(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := containerengine.ListNodePoolsRequest{CompartmentId: &s.CompartmentID}
	if s.OkeID != "" {
		req.ClusterId = &s.OkeID
	}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListNodePools(ctx, req)
	if err != nil {
		return nil, "", err
	}

	// One GetNodePool call per pool to resolve its Nodes — a
	// ListNodePools summary doesn't carry them — same N+1-per-row
	// pattern ExadbVmClusterResource.List() uses for its own DB nodes.
	rows := make([]Row, len(resp.Items))
	var wg sync.WaitGroup
	for i, np := range resp.Items {
		wg.Add(1)
		go func(i int, np containerengine.NodePoolSummary) {
			defer wg.Done()
			var nodes []containerengine.Node
			if full, err := client.GetNodePool(ctx, containerengine.GetNodePoolRequest{NodePoolId: np.Id}); err == nil {
				nodes = full.Nodes
			}
			rows[i] = Row{ID: deref(np.Id), Name: deref(np.Name), Raw: NodePoolRow{NodePoolSummary: np, Nodes: nodes}}
		}(i, np)
	}
	wg.Wait()

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
