package app

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	ltable "charm.land/lipgloss/v2/table"
	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/clients"
	"toci/internal/registry"
)

var drgRouteRuleHeaders = []string{"DESTINATION", "NEXT HOP", "ROUTE TYPE", "PROVENANCE"}

// drgRouteRuleRecords flattens a DRG route table's rules into plain
// records, resolving each rule's next-hop DRG attachment OCID to a name via
// attachmentNames (built once per fetch, see fetchDrgRouteRules) — the same
// "show a name, not an OCID" preference as drg_attachment.go's ATTACHED TO
// column.
func drgRouteRuleRecords(rules []core.DrgRouteRule, attachmentNames map[string]string) [][]string {
	records := make([][]string, 0, len(rules))
	for _, r := range rules {
		nextHop := "BLACKHOLE"
		if r.IsBlackhole == nil || !*r.IsBlackhole {
			nextHop = deref(r.NextHopDrgAttachmentId)
			if name, ok := attachmentNames[nextHop]; ok && name != "" {
				nextHop = name
			}
		}
		records = append(records, []string{
			deref(r.Destination),
			nextHop,
			string(r.RouteType),
			string(r.RouteProvenance),
		})
	}
	return records
}

// renderDrgRouteRules formats a DRG route table's rules as a bordered
// table, for the "v" key on a DRG Route Table row — same layout as
// renderSecurityRules/renderRouteRules, just with this view's own headers.
func renderDrgRouteRules(name string, records [][]string) string {
	t := ltable.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(ociBorder))).
		Headers(drgRouteRuleHeaders...).
		Rows(records...)

	title := fmt.Sprintf("%s — %d rules\n\n", name, len(records))
	return title + t.String()
}

type drgRouteRulesMsg struct {
	name    string
	records [][]string
	err     error
}

// fetchDrgRouteRules lists a DRG route table's rules plus every attachment
// on its parent DRG (for NEXT HOP name resolution) — two List calls, not
// one per rule, since a DRG typically has only a handful of attachments.
func fetchDrgRouteRules(ctx context.Context, factory *clients.Factory, region, drgID, routeTableID string) ([]core.DrgRouteRule, map[string]string, error) {
	client, err := factory.VirtualNetwork(region)
	if err != nil {
		return nil, nil, err
	}

	var rules []core.DrgRouteRule
	page := ""
	for {
		req := core.ListDrgRouteRulesRequest{DrgRouteTableId: &routeTableID}
		if page != "" {
			req.Page = &page
		}
		resp, err := client.ListDrgRouteRules(ctx, req)
		if err != nil {
			return nil, nil, err
		}
		rules = append(rules, resp.Items...)
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}

	names := map[string]string{}
	page = ""
	for {
		req := core.ListDrgAttachmentsRequest{DrgId: &drgID}
		if page != "" {
			req.Page = &page
		}
		resp, err := client.ListDrgAttachments(ctx, req)
		if err != nil {
			// Name resolution is a nice-to-have, not required to show the
			// rules themselves — fall back to raw OCIDs rather than fail
			// the whole view over it.
			break
		}
		for _, a := range resp.Items {
			names[deref(a.Id)] = deref(a.DisplayName)
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}

	return rules, names, nil
}

// buildDrgRouteRulesCmd fetches and formats row's rules for the "v" key —
// like NSG rules, a DRG route table's rules are their own resource behind
// ListDrgRouteRules, not embedded in Row.Raw, so this is a tea.Cmd rather
// than the synchronous securityRulesView/routeRulesView path.
func (m Model) buildDrgRouteRulesCmd(row registry.Row) tea.Cmd {
	rt, ok := row.Raw.(core.DrgRouteTable)
	if !ok {
		return nil
	}
	factory := m.factory
	region := m.scope.Region
	drgID := m.scope.DrgID
	name := deref(rt.DisplayName)
	routeTableID := row.ID
	return func() tea.Msg {
		rules, names, err := fetchDrgRouteRules(context.Background(), factory, region, drgID, routeTableID)
		if err != nil {
			return drgRouteRulesMsg{err: fmt.Errorf("list drg route rules: %w", err)}
		}
		return drgRouteRulesMsg{name: name, records: drgRouteRuleRecords(rules, names)}
	}
}
