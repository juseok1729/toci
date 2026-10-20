package app

import (
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/registry"
)

func TestOcidKind(t *testing.T) {
	cases := map[string]string{
		"ocid1.natgateway.oc1.x":          "nat",
		"ocid1.servicegateway.oc1.x":      "sgw",
		"ocid1.localpeeringgateway.oc1.x": "lpg",
		"ocid1.drg.oc1.x":                 "drg",
		"ocid1.internetgateway.oc1.x":     "igw",
	}
	for id, want := range cases {
		if got := ocidKind(id); got != want {
			t.Errorf("ocidKind(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestRenderTopologyHTMLEmbedsGraphSafely(t *testing.T) {
	g := topoGraph{Region: "ap-seoul-1", Nodes: []topoNode{{ID: "a", Label: "</script><b>x", Kind: "vcn"}}}
	out, err := renderTopologyHTML(g)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"__TOCI_DATA__"} {
		if strings.Contains(out, p) {
			t.Errorf("placeholder %s left in output", p)
		}
	}
	if strings.Contains(out, `"label":"</script>`) {
		t.Error("node label closed the script tag unescaped")
	}
}

func TestAddDrgTablesPutsVcnAttachmentTableOnDrgAndVcn(t *testing.T) {
	g := topoGraph{}
	g.addNode(topoNode{ID: "drg", Kind: "drg"})
	g.addNode(topoNode{ID: "vcn", Kind: "vcn"})
	rt, other := "rt1", "rt2"
	att := core.DrgAttachment{
		Id: ptr("att"), DrgId: ptr("drg"), DrgRouteTableId: &rt,
		NetworkDetails: core.VcnDrgAttachmentNetworkDetails{Id: ptr("vcn")},
	}
	g.addNode(topoNode{ID: "att2", Kind: "vpn", Label: "hq-vpn"})
	ipsec := core.DrgAttachment{
		Id: ptr("att2"), DrgId: ptr("drg"), DrgRouteTableId: &rt,
		NetworkDetails: core.IpsecTunnelDrgAttachmentNetworkDetails{Id: ptr("tun")},
	}
	atts := []registry.Row{
		{ID: "att", Name: "att-hub", Raw: registry.DrgAttachmentRow{DrgAttachment: att}},
		{ID: "att2", Name: "att-vpn", Raw: registry.DrgAttachmentRow{DrgAttachment: ipsec}},
	}
	rule := core.DrgRouteRule{Destination: ptr("10.0.0.0/8"), NextHopDrgAttachmentId: ptr("att")}
	tables := []drgTableData{
		{drgID: "drg", id: other, name: "other"},
		{drgID: "drg", id: rt, name: "vcn-rt", rules: []core.DrgRouteRule{rule}},
	}
	g.addDrgTables(atts, tables, map[string]string{"att2": "att2"})

	drg := g.Nodes[0].Tables
	if len(drg) != 2 || !drg[0].Open || drg[1].Open || drg[0].Rows[0][1] != "att-hub" {
		t.Errorf("DRG tables = %+v, want VCN-attachment table first/open with next hop resolved to att-hub, other collapsed", drg)
	}
	if far := g.Nodes[2].Tables; len(far) != 1 || !strings.Contains(far[0].Title, "via hq-vpn") {
		t.Errorf("IPSec node tables = %+v, want its attachment's DRG route table", far)
	}
	if v := g.Nodes[1].Tables; len(v) != 1 || !strings.Contains(v[0].Title, "via att-hub") {
		t.Errorf("VCN tables = %+v, want the attachment's DRG route table", v)
	}
}

func ptr(s string) *string { return &s }

func TestAddEdgeRoutesCollectsDestinationsPerAttachment(t *testing.T) {
	g := topoGraph{}
	g.addNamedEdge("vcn", "drg", "attached", "ok", "att-hub", "att")
	g.addNamedEdge("drg", "far", "ipsec", "ok", "att-vpn", "other")
	bh := true
	tables := []drgTableData{{rules: []core.DrgRouteRule{
		{Destination: ptr("10.0.0.0/16"), NextHopDrgAttachmentId: ptr("att")},
		{Destination: ptr("10.0.0.0/16"), NextHopDrgAttachmentId: ptr("att")}, // same CIDR in another rule: listed once
		{Destination: ptr("172.16.0.0/12"), NextHopDrgAttachmentId: ptr("att")},
		{Destination: ptr("192.168.0.0/16"), IsBlackhole: &bh}, // blackhole has no attachment
		{Destination: ptr("1.0.0.0/8"), NextHopDrgAttachmentId: ptr("other")},
	}}}
	rt := "rt-att"
	atts := []registry.Row{{ID: "att", Raw: registry.DrgAttachmentRow{DrgAttachment: core.DrgAttachment{DrgRouteTableId: &rt}}}}
	tables = append(tables, drgTableData{id: rt, rules: []core.DrgRouteRule{
		{Destination: ptr("0.0.0.0/0"), NextHopDrgAttachmentId: ptr("other")},
		{Destination: ptr("10.9.0.0/16"), NextHopDrgAttachmentId: ptr("other")},
	}})
	g.addEdgeRoutes(atts, tables)
	if got := g.Edges[0].Next["other"]; len(got) != 2 || got[0] != "0.0.0.0/0" {
		t.Errorf("Next[other] = %v, want the attachment's own route table rules that forward to the other attachment", got)
	}
	if got := strings.Join(g.Edges[0].Routes, ","); got != "10.0.0.0/16,172.16.0.0/12" {
		t.Errorf("routes = %q, want the attachment's two distinct destinations, sorted", got)
	}
}
