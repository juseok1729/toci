package app

import (
	"fmt"
	"strconv"

	"charm.land/lipgloss/v2"
	ltable "charm.land/lipgloss/v2/table"
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

// srcPortString/dstPortString read one direction's port range independent
// of the other (unlike the old combined "src:X dst:Y" PORTS column, OCI's
// own console shows Source/Destination Port Range as two separate ones) —
// "All" for TCP/UDP with no restriction on that side, blank for a
// protocol ports don't apply to at all (ICMP, "all").
func srcPortString(tcp *core.TcpOptions, udp *core.UdpOptions) string {
	return directionalPortString(tcp, udp, false)
}

func dstPortString(tcp *core.TcpOptions, udp *core.UdpOptions) string {
	return directionalPortString(tcp, udp, true)
}

func directionalPortString(tcp *core.TcpOptions, udp *core.UdpOptions, dest bool) string {
	var r *core.PortRange
	switch {
	case tcp != nil:
		if dest {
			r = tcp.DestinationPortRange
		} else {
			r = tcp.SourcePortRange
		}
	case udp != nil:
		if dest {
			r = udp.DestinationPortRange
		} else {
			r = udp.SourcePortRange
		}
	default:
		return ""
	}
	if r == nil {
		return "All"
	}
	return portRangeString(r)
}

// icmpTypeCodeString reads a rule's ICMP type/code — blank for any other
// protocol (ports and type/code are mutually exclusive: a rule is either
// TCP/UDP with ports, or ICMP/ICMPv6 with a type/code, never both). Per
// IcmpOptions' own doc, protocol ICMP/ICMPv6 with IcmpOptions omitted
// means every type and code is allowed, hence "All" there — as opposed to
// blank, which means the column doesn't apply to this rule at all.
func icmpTypeCodeString(protocol string, opts *core.IcmpOptions) string {
	if protocol != "1" && protocol != "58" {
		return ""
	}
	if opts == nil || opts.Type == nil {
		return "All"
	}
	if opts.Code == nil {
		return strconv.Itoa(*opts.Type)
	}
	return strconv.Itoa(*opts.Type) + ":" + strconv.Itoa(*opts.Code)
}

var securityRuleHeaders = []string{"DIR", "STATELESS", "SOURCE", "PROTOCOL", "SOURCE PORT", "DEST PORT", "TYPE AND CODE", "DESCRIPTION"}

// securityRuleRecords flattens a security list's ingress/egress rules
// (ingress rows first, then egress) into plain records — shared by both
// the rendered table view ("v") and its CSV export ("e" from that view),
// so the two can never drift apart.
func securityRuleRecords(sl core.SecurityList) [][]string {
	records := make([][]string, 0, len(sl.IngressSecurityRules)+len(sl.EgressSecurityRules))
	for _, r := range sl.IngressSecurityRules {
		proto := deref(r.Protocol)
		records = append(records, []string{
			"INGRESS",
			yesNo(r.IsStateless),
			deref(r.Source),
			protocolName(proto),
			srcPortString(r.TcpOptions, r.UdpOptions),
			dstPortString(r.TcpOptions, r.UdpOptions),
			icmpTypeCodeString(proto, r.IcmpOptions),
			deref(r.Description),
		})
	}
	for _, r := range sl.EgressSecurityRules {
		proto := deref(r.Protocol)
		records = append(records, []string{
			"EGRESS",
			yesNo(r.IsStateless),
			deref(r.Destination),
			protocolName(proto),
			srcPortString(r.TcpOptions, r.UdpOptions),
			dstPortString(r.TcpOptions, r.UdpOptions),
			icmpTypeCodeString(proto, r.IcmpOptions),
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
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(ociBorder))).
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
