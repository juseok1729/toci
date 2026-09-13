package app

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/clients"
	"toci/internal/registry"
)

// nsgRuleRecords flattens an NSG's security rules into the same plain
// record shape a Security List's rules use (see securityRuleHeaders) —
// core.SecurityRule (an NSG rule) and core.IngressSecurityRule/
// EgressSecurityRule (a Security List's) are different SDK types, but
// carry the same fields, just under one Direction instead of two lists.
func nsgRuleRecords(rules []core.SecurityRule) [][]string {
	records := make([][]string, 0, len(rules))
	for _, r := range rules {
		endpoint := deref(r.Destination)
		if r.Direction == core.SecurityRuleDirectionIngress {
			endpoint = deref(r.Source)
		}
		records = append(records, []string{
			string(r.Direction),
			protocolName(deref(r.Protocol)),
			endpoint,
			portsString(r.TcpOptions, r.UdpOptions),
			yesNo(r.IsStateless),
			deref(r.Description),
		})
	}
	return records
}

type nsgRulesMsg struct {
	name    string
	records [][]string
	err     error
}

// fetchNsgRules lists every security rule attached to an NSG — unlike a
// Security List (whose ingress/egress rules are embedded fields on the
// list itself, already sitting in Row.Raw), an NSG's rules are their own
// resource behind ListNetworkSecurityGroupSecurityRules.
func fetchNsgRules(ctx context.Context, factory *clients.Factory, region, nsgID string) ([]core.SecurityRule, error) {
	client, err := factory.VirtualNetwork(region)
	if err != nil {
		return nil, err
	}
	var rules []core.SecurityRule
	page := ""
	for {
		req := core.ListNetworkSecurityGroupSecurityRulesRequest{NetworkSecurityGroupId: &nsgID}
		if page != "" {
			req.Page = &page
		}
		resp, err := client.ListNetworkSecurityGroupSecurityRules(ctx, req)
		if err != nil {
			return nil, err
		}
		rules = append(rules, resp.Items...)
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return rules, nil
}

// buildNsgRulesCmd fetches and formats row's rules for the "v" key — unlike
// securityRulesView/routeRulesView (synchronous, since their rules are
// already embedded in Row.Raw), this needs its own network call, so it's a
// tea.Cmd like buildResourceMap/buildVcnDiagram.
func (m Model) buildNsgRulesCmd(row registry.Row) tea.Cmd {
	nsg, ok := row.Raw.(core.NetworkSecurityGroup)
	if !ok {
		return nil
	}
	factory := m.factory
	region := m.scope.Region
	name := deref(nsg.DisplayName)
	nsgID := row.ID
	return func() tea.Msg {
		rules, err := fetchNsgRules(context.Background(), factory, region, nsgID)
		if err != nil {
			return nsgRulesMsg{err: fmt.Errorf("list nsg rules: %w", err)}
		}
		return nsgRulesMsg{name: name, records: nsgRuleRecords(rules)}
	}
}
