package registry

import (
	"context"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/loadbalancer"
	"toci/internal/clients"
)

type LoadBalancerResource struct {
	factory *clients.Factory
}

func NewLoadBalancerResource(f *clients.Factory) *LoadBalancerResource {
	return &LoadBalancerResource{factory: f}
}

func (r *LoadBalancerResource) Key() string   { return "lb" }
func (r *LoadBalancerResource) Label() string { return "Load Balancers" }

func anySubnetIn(subnetIDs []string, allow map[string]bool) bool {
	for _, id := range subnetIDs {
		if allow[id] {
			return true
		}
	}
	return false
}

func (r *LoadBalancerResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.LoadBalancer).DisplayName)
		}},
		{Header: "SHAPE", Width: 14, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.LoadBalancer).ShapeName)
		}},
		{Header: "IPS", Width: 24, Get: func(row Row) string {
			lb := row.Raw.(loadbalancer.LoadBalancer)
			ips := make([]string, 0, len(lb.IpAddresses))
			for _, ip := range lb.IpAddresses {
				ips = append(ips, deref(ip.IpAddress))
			}
			return strings.Join(ips, ", ")
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return string(row.Raw.(loadbalancer.LoadBalancer).LifecycleState)
		}},
	}
}

func (r *LoadBalancerResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.LoadBalancer(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := loadbalancer.ListLoadBalancersRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListLoadBalancers(ctx, req)
	if err != nil {
		return nil, "", err
	}

	// ListLoadBalancers has no VcnId filter, but each LB already lists its
	// own subnet IDs — no join against another list needed, unlike Instance.
	var allowSubnets map[string]bool
	if s.VcnID != "" {
		vnClient, err := r.factory.VirtualNetwork(s.Region)
		if err != nil {
			return nil, "", err
		}
		allowSubnets, err = vcnSubnetIDs(ctx, vnClient, s.CompartmentID, s.VcnID)
		if err != nil {
			return nil, "", err
		}
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, lb := range resp.Items {
		if allowSubnets != nil && !anySubnetIn(lb.SubnetIds, allowSubnets) {
			continue
		}
		rows = append(rows, Row{ID: deref(lb.Id), Name: deref(lb.DisplayName), Raw: lb})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
