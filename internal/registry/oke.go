package registry

import (
	"context"
	"time"

	"github.com/oracle/oci-go-sdk/v65/containerengine"
	"toci/internal/clients"
)

type OkeResource struct {
	factory *clients.Factory
}

func NewOkeResource(f *clients.Factory) *OkeResource {
	return &OkeResource{factory: f}
}

func (r *OkeResource) Key() string   { return "oke" }
func (r *OkeResource) Label() string { return "OKE Clusters" }

func (r *OkeResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(containerengine.ClusterSummary).Name)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(containerengine.ClusterSummary).LifecycleState)
		}},
		{Header: "TYPE", Width: 16, Get: func(row Row) string {
			return stateLabel(row.Raw.(containerengine.ClusterSummary).Type)
		}},
		{Header: "K8S VERSION", Width: 14, Get: func(row Row) string {
			return deref(row.Raw.(containerengine.ClusterSummary).KubernetesVersion)
		}},
		{Header: "PUBLIC ENDPOINT", Width: 20, Get: func(row Row) string {
			ep := row.Raw.(containerengine.ClusterSummary).Endpoints
			if ep == nil {
				return ""
			}
			return deref(ep.PublicEndpoint)
		}},
		{Header: "PRIVATE ENDPOINT", Width: 20, Get: func(row Row) string {
			ep := row.Raw.(containerengine.ClusterSummary).Endpoints
			if ep == nil {
				return ""
			}
			return deref(ep.PrivateEndpoint)
		}},
	}
}

func (r *OkeResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.ContainerEngine(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := containerengine.ListClustersRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListClusters(ctx, req)
	if err != nil {
		return nil, "", err
	}

	// ListClusters has no VcnId filter param (unlike ListSubnets/ListInstances),
	// so a VCN scope is applied client-side against each cluster's own VcnId.
	rows := make([]Row, 0, len(resp.Items))
	for _, cl := range resp.Items {
		if s.VcnID != "" && deref(cl.VcnId) != s.VcnID {
			continue
		}
		var created time.Time
		if cl.Metadata != nil {
			created = timeOf(cl.Metadata.TimeCreated)
		}
		rows = append(rows, Row{ID: deref(cl.Id), Name: deref(cl.Name), TimeCreated: created, Raw: cl})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
