package app

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/lipgloss"
	ltable "github.com/charmbracelet/lipgloss/table"
	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/registry"
)

// protocolName maps the IANA protocol numbers OCI security rules use to
// their common names — the raw list only ever shows "6", "17", etc.
func protocolName(proto string) string {
	switch proto {
	case "all":
		return "ALL"
	case "1":
		return "ICMP"
	case "6":
		return "TCP"
	case "17":
		return "UDP"
	case "58":
		return "ICMPv6"
	default:
		return proto
	}
}

func portRangeString(r *core.PortRange) string {
	if r == nil {
		return ""
	}
	if r.Min != nil && r.Max != nil && *r.Min == *r.Max {
		return strconv.Itoa(*r.Min)
	}
	minS, maxS := "", ""
	if r.Min != nil {
		minS = strconv.Itoa(*r.Min)
	}
	if r.Max != nil {
		maxS = strconv.Itoa(*r.Max)
	}
	return minS + "-" + maxS
}

func portsString(tcp *core.TcpOptions, udp *core.UdpOptions) string {
	var src, dst *core.PortRange
	switch {
	case tcp != nil:
		src, dst = tcp.SourcePortRange, tcp.DestinationPortRange
	case udp != nil:
		src, dst = udp.SourcePortRange, udp.DestinationPortRange
	default:
		return "ALL"
	}
	if src == nil && dst == nil {
		return "ALL"
	}
	s := portRangeString(dst)
	if s == "" {
		s = "ALL"
	}
	if src != nil {
		s = "src:" + portRangeString(src) + " dst:" + s
	}
	return s
}

var securityRuleHeaders = []string{"DIR", "PROTOCOL", "SOURCE/DEST", "PORTS", "STATELESS", "DESCRIPTION"}

// securityRuleRecords flattens a security list's ingress/egress rules
// (ingress rows first, then egress) into plain records — shared by both
// the rendered table view ("v") and its CSV export ("e" from that view),
// so the two can never drift apart.
func securityRuleRecords(sl core.SecurityList) [][]string {
	records := make([][]string, 0, len(sl.IngressSecurityRules)+len(sl.EgressSecurityRules))
	for _, r := range sl.IngressSecurityRules {
		records = append(records, []string{
			"INGRESS",
			protocolName(deref(r.Protocol)),
			deref(r.Source),
			portsString(r.TcpOptions, r.UdpOptions),
			yesNo(r.IsStateless),
			deref(r.Description),
		})
	}
	for _, r := range sl.EgressSecurityRules {
		records = append(records, []string{
			"EGRESS",
			protocolName(deref(r.Protocol)),
			deref(r.Destination),
			portsString(r.TcpOptions, r.UdpOptions),
			yesNo(r.IsStateless),
			deref(r.Description),
		})
	}
	return records
}

// renderSecurityRules formats a security list's ingress/egress rules as a
// bordered table, for the "v" key on a Security List row — the plain YAML
// detail view buries these in deeply nested, mostly-null fields that are
// hard to scan at a glance.
func renderSecurityRules(name string, records [][]string) string {
	t := ltable.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		Headers(securityRuleHeaders...).
		Rows(records...)

	title := fmt.Sprintf("%s — %d rules\n\n", name, len(records))
	return title + t.String()
}

// securityRulesView builds the rules table + CSV records for row if it's a
// Security List, so model.go can drive both "v" and "e" without importing
// the SDK's core package just for one type assertion.
func securityRulesView(row registry.Row) (rendered string, records [][]string, name string, ok bool) {
	sl, ok := row.Raw.(core.SecurityList)
	if !ok {
		return "", nil, "", false
	}
	name = deref(sl.DisplayName)
	records = securityRuleRecords(sl)
	return renderSecurityRules(name, records), records, name, true
}

func yesNo(b *bool) string {
	if b != nil && *b {
		return "yes"
	}
	return "no"
}
