package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	ltable "charm.land/lipgloss/v2/table"
	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/registry"
)

// routeTargetKindNames maps a route rule target's OCID resource-type
// segment to a friendly label — core.RouteRule has no separate "target
// type" field, just the raw NetworkEntityId OCID (e.g.
// "ocid1.internetgateway.oc1..<unique>").
var routeTargetKindNames = map[string]string{
	"internetgateway":     "Internet Gateway",
	"natgateway":          "NAT Gateway",
	"servicegateway":      "Service Gateway",
	"drg":                 "DRG",
	"localpeeringgateway": "Local Peering GW",
	"privateip":           "Private IP",
}

// routeTargetKind extracts ocid's resource-type segment
// ("ocid1.<type>.<realm>...") and looks it up in routeTargetKindNames,
// falling back to the raw segment for a target kind not listed there
// (still more readable than the full OCID) or "-" if ocid is empty/malformed.
func routeTargetKind(ocid string) string {
	parts := strings.SplitN(ocid, ".", 3)
	if len(parts) < 2 || parts[1] == "" {
		return "-"
	}
	if name, ok := routeTargetKindNames[parts[1]]; ok {
		return name
	}
	return parts[1]
}

var routeRuleHeaders = []string{"DESTINATION", "TARGET", "ROUTE TYPE", "DESCRIPTION"}

// routeRuleRecords flattens a route table's rules into plain records —
// shared by the rendered table view ("v") and its CSV export ("e" from
// that view), so the two can never drift apart.
func routeRuleRecords(rt core.RouteTable) [][]string {
	records := make([][]string, 0, len(rt.RouteRules))
	for _, r := range rt.RouteRules {
		routeType := string(r.RouteType)
		if routeType == "" {
			// Doc: "a route rule can be STATIC if manually added ... LOCAL
			// if added by OCI" — every rule is conceptually one of the
			// two, an empty field here just means an older API response
			// didn't set it. STATIC (the manually-added case) is the
			// far more common one to default to.
			routeType = string(core.RouteRuleRouteTypeStatic)
		}
		records = append(records, []string{
			deref(r.Destination),
			routeTargetKind(deref(r.NetworkEntityId)),
			routeType,
			deref(r.Description),
		})
	}
	return records
}

// renderRouteRules formats a route table's rules as a bordered table, for
// the "v" key on a Route Table row — the plain YAML detail view buries
// these in a nested RouteRules list that's hard to scan at a glance.
func renderRouteRules(name string, records [][]string) string {
	t := ltable.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(ociBorder))).
		Headers(routeRuleHeaders...).
		Rows(records...)

	title := fmt.Sprintf("%s — %d rules\n\n", name, len(records))
	return title + t.String()
}

// routeRulesView builds the rules table + CSV records for row if it's a
// Route Table, so model.go can drive both "v" and "e" without importing
// the SDK's core package just for one type assertion.
func routeRulesView(row registry.Row) (rendered string, records [][]string, name string, ok bool) {
	rt, ok := row.Raw.(registry.RouteTableRow)
	if !ok {
		return "", nil, "", false
	}
	name = deref(rt.DisplayName)
	records = routeRuleRecords(rt.RouteTable)
	return renderRouteRules(name, records), records, name, true
}
