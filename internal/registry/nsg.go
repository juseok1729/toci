package registry

import (
	"context"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

// NsgRow adds the NSG's own rule count (fetched via
// ListNetworkSecurityGroupSecurityRules — a NetworkSecurityGroup itself
// carries no count) so the table doesn't need a separate "v" press just
// to see how many rules it has.
type NsgRow struct {
	core.NetworkSecurityGroup
	RuleCount int
}

type NsgResource struct {
	factory *clients.Factory
}

func NewNsgResource(f *clients.Factory) *NsgResource {
	return &NsgResource{factory: f}
}

func (r *NsgResource) Key() string   { return "nsg" }
func (r *NsgResource) Label() string { return "NSG" }

func (r *NsgResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(NsgRow).DisplayName)
		}},
		{Header: "RULES", Width: 6, Get: func(row Row) string {
			return itoa(row.Raw.(NsgRow).RuleCount)
		}},
	}
}

// countNsgRules pages through an NSG's own security rules just to count
// them — same list call buildNsgRulesCmd (internal/app/nsg_rules.go) uses
// to render them for "v", but this only needs the length, not the rules
// themselves. A failed call reads as 0 rather than failing the whole row.
func countNsgRules(ctx context.Context, client core.VirtualNetworkClient, nsgID *string) int {
	count := 0
	page := ""
	for {
		req := core.ListNetworkSecurityGroupSecurityRulesRequest{NetworkSecurityGroupId: nsgID}
		if page != "" {
			req.Page = &page
		}
		resp, err := client.ListNetworkSecurityGroupSecurityRules(ctx, req)
		if err != nil {
			return count
		}
		count += len(resp.Items)
		if resp.OpcNextPage == nil {
			return count
		}
		page = *resp.OpcNextPage
	}
}

func (r *NsgResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListNetworkSecurityGroupsRequest{CompartmentId: &s.CompartmentID}
	if s.VcnID != "" {
		req.VcnId = &s.VcnID
	}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListNetworkSecurityGroups(ctx, req)
	if err != nil {
		return nil, "", err
	}

	// One ListNetworkSecurityGroupSecurityRules call per NSG to resolve its
	// rule count — same N+1-per-row pattern ExadbVmClusterResource.List()
	// uses for its own DB nodes.
	rows := make([]Row, len(resp.Items))
	var wg sync.WaitGroup
	for i, n := range resp.Items {
		wg.Add(1)
		go func(i int, n core.NetworkSecurityGroup) {
			defer wg.Done()
			count := countNsgRules(ctx, client, n.Id)
			rows[i] = Row{ID: deref(n.Id), Name: deref(n.DisplayName), TimeCreated: timeOf(n.TimeCreated), Raw: NsgRow{NetworkSecurityGroup: n, RuleCount: count}}
		}(i, n)
	}
	wg.Wait()

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
