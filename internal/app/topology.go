package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/clients"
	"toci/internal/registry"
)

//go:embed topology/topology.html
var topologyTemplate string

type topologyMsg struct {
	path   string
	err    error
	failed int // connections whose details could not be looked up
}

type topoNode struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	Comp   string `json:"comp"`
	// Items are extra lines for the page's info panel (a VCN's subnets,
	// a route table's users); subnets are not graph nodes themselves.
	ItemsTitle string      `json:"itemsTitle,omitempty"`
	Items      []string    `json:"items,omitempty"`
	Tables     []topoTable `json:"tables,omitempty"`
	// Sub is the caption under a connection's name (an IPSec connection's
	// "<cpe ip> / <routes>"); Cpe is the on-prem device drawn in the
	// External column.
	Sub string   `json:"sub,omitempty"`
	Cpe *topoCPE `json:"cpe,omitempty"`
	// Props are the "Resource summary" rows after OCID and Name.
	Props []topoKV `json:"props,omitempty"`
	// Err says why a connection's details (CPE, circuit, peer) could not be looked up.
	Err string `json:"err,omitempty"`
	// Peer / PeerStatus: a local peering gateway's peer LPG (OCID) and the peering status.
	Peer       string `json:"peer,omitempty"`
	PeerStatus string `json:"peerStatus,omitempty"`
}

// topoKV is one Resource summary row; T picks the rendering ("state" gets a
// coloured dot, "link" is drawn like the console's resource links).
type topoKV struct {
	K string `json:"k"`
	V string `json:"v"`
	T string `json:"t,omitempty"`
}

type topoCPE struct {
	Kind string `json:"kind"` // "cpe" or "provider"
	Name string `json:"name"`
	IP   string `json:"ip,omitempty"`
}

// topoTable is a rules table shown in the info panel (a route table's
// rules, a DRG route table's rules).
type topoTable struct {
	Title   string     `json:"title"`
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
	Open    bool       `json:"open"`
}

type topoEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Rel    string `json:"rel"`
	State  string `json:"state"`
	Name   string `json:"name,omitempty"` // the DRG attachment's display name
	Att    string `json:"att,omitempty"`  // the DRG attachment's OCID
	// Routes are the destinations the DRG sends out through this attachment
	// (rules whose next hop is it), drawn as the Visualizer's route pills.
	Routes []string `json:"routes,omitempty"`
	// RoutesIn are the destinations the attachment's own DRG route table sends
	// traffic to. Next breaks those down by next-hop attachment ID: the thin
	// lines (and route pills) the Visualizer draws inside the DRG are exactly
	// these attachment-to-attachment hops.
	RoutesIn []string            `json:"routesIn,omitempty"`
	Next     map[string][]string `json:"next,omitempty"`
	Props    []topoKV            `json:"props,omitempty"`
}

type topoGraph struct {
	Region string     `json:"region"`
	Nodes  []topoNode `json:"nodes"`
	Edges  []topoEdge `json:"edges"`
}

func (g *topoGraph) hasNode(id string) bool {
	for _, n := range g.Nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func (g *topoGraph) hasEdge(src, dst string) bool {
	for _, e := range g.Edges {
		if e.Source == src && e.Target == dst {
			return true
		}
	}
	return false
}

func (g *topoGraph) addTable(id string, t topoTable) {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			g.Nodes[i].Tables = append(g.Nodes[i].Tables, t)
			return
		}
	}
}

func (g *topoGraph) setItems(id, title string, items []string) {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			g.Nodes[i].ItemsTitle, g.Nodes[i].Items = title, items
			return
		}
	}
}

// addDrgTables fills the info panel's DRG route tables: a DRG node lists
// its route tables (the ones its VCN attachments use open, the rest
// collapsed), and a VCN node shows the DRG route table of each of its
// attachments — the table that governs traffic entering the DRG from it.
func (g *topoGraph) addDrgTables(atts []registry.Row, tables []drgTableData, farNode map[string]string) {
	attName := map[string]string{}
	vcnRT := map[string]bool{} // DRG route table IDs used by a VCN attachment
	for _, row := range atts {
		attName[row.ID] = row.Name
		if a, ok := row.Raw.(registry.DrgAttachmentRow); ok && a.DrgRouteTableId != nil {
			if _, isVcn := a.NetworkDetails.(core.VcnDrgAttachmentNetworkDetails); isVcn || a.NetworkDetails == nil {
				vcnRT[*a.DrgRouteTableId] = true
			}
		}
	}
	byID := map[string]topoTable{}
	for _, t := range tables {
		byID[t.id] = topoTable{Rows: drgRouteRuleRecords(t.rules, attName), Headers: drgRouteRuleHeaders}
	}
	// VCN-attachment tables first so they sit open at the top of a DRG's panel.
	for _, pass := range []bool{true, false} {
		for _, t := range tables {
			if vcnRT[t.id] != pass {
				continue
			}
			bt := byID[t.id]
			title := fmt.Sprintf("DRG RT: %s (%d rules)", t.name, len(bt.Rows))
			if pass {
				title += " · VCN attachments"
			}
			g.addTable(t.drgID, topoTable{Title: title, Headers: bt.Headers, Rows: bt.Rows, Open: pass})
		}
	}
	tableName := map[string]string{}
	for _, t := range tables {
		tableName[t.id] = t.name
	}
	for _, row := range atts {
		a, ok := row.Raw.(registry.DrgAttachmentRow)
		if !ok || a.DrgRouteTableId == nil {
			continue
		}
		bt, ok := byID[*a.DrgRouteTableId]
		if !ok {
			continue
		}
		// IPSec/FastConnect/RPC attachments land on their connection node (several
		// IPSec tunnels share one); a VCN attachment's table goes on the VCN.
		target := farNode[row.ID]
		if _, isVcn := a.NetworkDetails.(core.VcnDrgAttachmentNetworkDetails); isVcn || a.NetworkDetails == nil {
			target = deref(a.VcnId)
			if nd, ok := a.NetworkDetails.(core.VcnDrgAttachmentNetworkDetails); ok {
				target = deref(nd.Id)
			}
		}
		if !g.hasNode(target) {
			continue
		}
		via := row.Name
		if farNode[row.ID] != "" {
			via = g.labelOf(target)
		}
		g.addTableOnce(target, topoTable{
			Title:   fmt.Sprintf("DRG RT: %s (%d rules) · via %s", tableName[*a.DrgRouteTableId], len(bt.Rows), via),
			Headers: bt.Headers, Rows: bt.Rows, Open: true,
		})
	}
}

func (g *topoGraph) labelOf(id string) string {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n.Label
		}
	}
	return id
}

// addTableOnce skips a table whose title the node already shows (two tunnels
// of one IPSec connection use the same DRG route table).
func (g *topoGraph) addTableOnce(id string, t topoTable) {
	for _, n := range g.Nodes {
		if n.ID == id {
			for _, x := range n.Tables {
				if x.Title == t.Title {
					return
				}
			}
		}
	}
	g.addTable(id, t)
}

func (g *topoGraph) compOf(id string) string {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n.Comp
		}
	}
	return ""
}

func (g *topoGraph) addNode(n topoNode) {
	if !g.hasNode(n.ID) {
		g.Nodes = append(g.Nodes, n)
	}
}

func (g *topoGraph) addEdge(src, dst, rel, state string) {
	g.Edges = append(g.Edges, topoEdge{ID: fmt.Sprintf("e%d", len(g.Edges)), Source: src, Target: dst, Rel: rel, State: state})
}

func (g *topoGraph) addNamedEdge(src, dst, rel, state, name, att string) {
	g.addEdge(src, dst, rel, state)
	e := &g.Edges[len(g.Edges)-1]
	e.Name, e.Att = name, att
}

// addEdgeRoutes fills each attachment edge's route pill and inner DRG lines.
// For attachment A: Routes = destinations of every DRG route rule whose next
// hop is A (what the DRG sends out through it); RoutesIn = destinations in A's
// own DRG route table (what traffic entering the DRG from A is routed to), and
// Next = the same rules grouped by next-hop attachment.
func (g *topoGraph) addEdgeRoutes(atts []registry.Row, tables []drgTableData) {
	out := map[string]map[string]bool{}
	byTable := map[string]drgTableData{}
	for _, t := range tables {
		byTable[t.id] = t
		for _, r := range t.rules {
			if r.IsBlackhole != nil && *r.IsBlackhole {
				continue
			}
			att := deref(r.NextHopDrgAttachmentId)
			if out[att] == nil {
				out[att] = map[string]bool{}
			}
			out[att][deref(r.Destination)] = true
		}
	}
	tableOf := map[string]string{} // attachment ID -> its DRG route table ID
	for _, row := range atts {
		if a, ok := row.Raw.(registry.DrgAttachmentRow); ok && a.DrgRouteTableId != nil {
			tableOf[row.ID] = *a.DrgRouteTableId
		}
	}
	for i := range g.Edges {
		e := &g.Edges[i]
		for d := range out[e.Att] {
			e.Routes = append(e.Routes, d)
		}
		sort.Strings(e.Routes)
		in, next := map[string]bool{}, map[string]map[string]bool{}
		for _, r := range byTable[tableOf[e.Att]].rules {
			if r.IsBlackhole != nil && *r.IsBlackhole {
				continue
			}
			in[deref(r.Destination)] = true
			if nh := deref(r.NextHopDrgAttachmentId); nh != "" && nh != e.Att {
				if next[nh] == nil {
					next[nh] = map[string]bool{}
				}
				next[nh][deref(r.Destination)] = true
			}
		}
		for d := range in {
			e.RoutesIn = append(e.RoutesIn, d)
		}
		sort.Strings(e.RoutesIn)
		for nh, ds := range next {
			if e.Next == nil {
				e.Next = map[string][]string{}
			}
			for d := range ds {
				e.Next[nh] = append(e.Next[nh], d)
			}
			sort.Strings(e.Next[nh])
		}
	}
}

// addEdgeProps builds each attachment's "Resource summary" rows (what the
// console shows when you click the link): compartment, DRG, DRG route table,
// ECMP, the attached VCN and its route table, creation time.
func (g *topoGraph) addEdgeProps(ctx context.Context, factory *clients.Factory, region string, atts []registry.Row, tables []drgTableData, compPath map[string]string) {
	tableByID := map[string]drgTableData{}
	for _, t := range tables {
		tableByID[t.id] = t
	}
	byAtt := map[string]registry.DrgAttachmentRow{}
	rtIDs := map[string]bool{}
	for _, row := range atts {
		if a, ok := row.Raw.(registry.DrgAttachmentRow); ok {
			byAtt[row.ID] = a
			if a.RouteTableId != nil {
				rtIDs[*a.RouteTableId] = true
			}
		}
	}
	// The VCN route table an attachment uses (VCN ingress routing) may not be one any subnet uses, so look it up by ID.
	rtName := map[string]string{}
	if client, err := factory.VirtualNetwork(region); err == nil {
		var mu sync.Mutex
		var wg sync.WaitGroup
		for id := range rtIDs {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				if r, err := client.GetRouteTable(ctx, core.GetRouteTableRequest{RtId: &id}); err == nil {
					mu.Lock()
					rtName[id] = deref(r.DisplayName)
					mu.Unlock()
				}
			}(id)
		}
		wg.Wait()
	}
	conn := map[string]string{"ipsec": "IPSec connection", "fastconnect": "Virtual circuit", "rpc": "Remote peering connection"}
	for i := range g.Edges {
		e := &g.Edges[i]
		a, ok := byAtt[e.Att]
		if !ok {
			continue
		}
		var p []topoKV
		add := func(k, v, t string) {
			if v != "" {
				p = append(p, topoKV{K: k, V: v, T: t})
			}
		}
		add("Compartment", compPath[deref(a.CompartmentId)], "")
		add("DRG", g.labelOf(deref(a.DrgId)), "link")
		if t, ok := tableByID[deref(a.DrgRouteTableId)]; ok {
			add("DRG Route Table", t.name, "link")
			ecmp := "Disabled"
			if t.ecmp {
				ecmp = "Enabled"
			}
			add("ECMP", ecmp, "")
		}
		if e.Rel == "attached" {
			add("Virtual Cloud Network", g.labelOf(e.Source), "link")
			add("VCN Route Table", rtName[deref(a.RouteTableId)], "link")
		} else {
			add(conn[e.Rel], g.labelOf(e.Target), "link")
		}
		if a.LifecycleState != core.DrgAttachmentLifecycleStateAttached {
			add("State", string(a.LifecycleState), "state")
		}
		if a.TimeCreated != nil {
			add("Created", consoleTime(a.TimeCreated.Time), "")
		}
		e.Props = p
	}
}

// baseProps are the Compartment / State / Created rows every resource summary has.
func baseProps(comp, state string, created *common.SDKTime) []topoKV {
	var p []topoKV
	if comp != "" {
		p = append(p, topoKV{K: "Compartment", V: comp})
	}
	if state != "" {
		p = append(p, topoKV{K: "State", V: strings.ToUpper(state[:1]) + strings.ToLower(state[1:]), T: "state"})
	}
	if created != nil {
		p = append(p, topoKV{K: "Created", V: consoleTime(created.Time)})
	}
	return p
}

// consoleTime formats like the OCI console: "Mon, Oct 19, 2026, 18:04:34 GMT+9".
func consoleTime(t time.Time) string {
	t = t.Local()
	_, off := t.Zone()
	return t.Format("Mon, Jan 2, 2006, 15:04:05") + fmt.Sprintf(" GMT%+d", off/3600)
}

// ocidKind maps a route target's OCID to the topology node kind by its
// resource-type segment ("ocid1.natgateway.oc1..."), since the resource
// map's gatewayEntity doesn't carry the gateway kind.
func ocidKind(ocid string) string {
	parts := strings.Split(ocid, ".")
	if len(parts) < 2 {
		return "igw"
	}
	switch parts[1] {
	case "natgateway":
		return "nat"
	case "servicegateway":
		return "sgw"
	case "localpeeringgateway":
		return "lpg"
	case "drg":
		return "drg"
	default:
		return "igw"
	}
}

// attachmentNode classifies a non-VCN DRG attachment's far end and picks its
// node ID: an IPSec tunnel collapses into its parent connection (a connection
// normally has two tunnels, drawn as two links to one box); the rest are
// identified by the virtual circuit / remote peering connection itself.
func attachmentNode(nd core.DrgAttachmentNetworkDetails, attachmentID string) (kind, rel, nodeID string) {
	switch d := nd.(type) {
	case core.IpsecTunnelDrgAttachmentNetworkDetails:
		return "vpn", "ipsec", firstNonEmpty(deref(d.IpsecConnectionId), attachmentID)
	case core.VirtualCircuitDrgAttachmentNetworkDetails:
		return "fc", "fastconnect", firstNonEmpty(deref(d.Id), attachmentID)
	case core.RemotePeeringConnectionDrgAttachmentNetworkDetails:
		return "rpc", "rpc", firstNonEmpty(deref(d.Id), attachmentID)
	}
	return "", "", ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// farRef is one distinct IPSec connection / virtual circuit / remote peering
// connection that needs its details looked up.
type farRef struct {
	nodeID, kind string
	nd           core.DrgAttachmentNetworkDetails
}

// describeConnections fills in what the Visualizer shows for each far end:
// an IPSec connection's name, CPE (name + public IP) and static routes; a
// virtual circuit's provider and bandwidth; a remote peering connection's
// peer region. A failed lookup just leaves the node with its generic label.
func (g *topoGraph) describeConnections(ctx context.Context, factory *clients.Factory, region string, far []farRef, compPath map[string]string) {
	client, err := factory.VirtualNetwork(region)
	if err != nil {
		return
	}
	type result struct {
		label, sub, err string
		cpe             *topoCPE
		props           []topoKV
	}
	res := make([]result, len(far))
	var wg sync.WaitGroup
	for i, f := range far {
		wg.Add(1)
		go func(i int, f farRef) {
			defer wg.Done()
			switch d := f.nd.(type) {
			case core.IpsecTunnelDrgAttachmentNetworkDetails:
				if d.IpsecConnectionId == nil {
					res[i].err = "the DRG attachment has no ipsecConnectionId"
					return
				}
				c, err := client.GetIPSecConnection(ctx, core.GetIPSecConnectionRequest{IpscId: d.IpsecConnectionId})
				if err != nil {
					res[i].err = "GetIPSecConnection: " + err.Error()
					return
				}
				r := result{label: deref(c.DisplayName)}
				routes := "BGP"
				if len(c.StaticRoutes) > 0 {
					routes = strings.Join(c.StaticRoutes, ", ")
				}
				r.sub = routes
				r.props = baseProps(compPath[deref(c.CompartmentId)], string(c.LifecycleState), c.TimeCreated)
				if len(c.StaticRoutes) > 0 {
					r.props = append(r.props, topoKV{K: "Static Route CIDR", V: routes})
				}
				if cpe, err := client.GetCpe(ctx, core.GetCpeRequest{CpeId: c.CpeId}); err == nil {
					r.cpe = &topoCPE{Kind: "cpe", Name: deref(cpe.DisplayName), IP: deref(cpe.IpAddress)}
					r.sub = deref(cpe.IpAddress) + " / " + routes
				} else {
					r.err = "GetCpe: " + err.Error()
				}
				res[i] = r
			case core.VirtualCircuitDrgAttachmentNetworkDetails:
				vc, err := client.GetVirtualCircuit(ctx, core.GetVirtualCircuitRequest{VirtualCircuitId: d.Id})
				if err != nil {
					res[i].err = "GetVirtualCircuit: " + err.Error()
					return
				}
				r := result{label: deref(vc.DisplayName), sub: strings.TrimSpace(deref(vc.BandwidthShapeName))}
				if p := deref(vc.ProviderName); p != "" {
					r.cpe = &topoCPE{Kind: "provider", Name: p}
				}
				res[i] = r
			case core.RemotePeeringConnectionDrgAttachmentNetworkDetails:
				rpc, err := client.GetRemotePeeringConnection(ctx, core.GetRemotePeeringConnectionRequest{RemotePeeringConnectionId: d.Id})
				if err != nil {
					res[i].err = "GetRemotePeeringConnection: " + err.Error()
					return
				}
				r := result{label: deref(rpc.DisplayName)}
				if p := deref(rpc.PeerRegionName); p != "" {
					r.sub = "peer: " + p
				}
				res[i] = r
			}
		}(i, f)
	}
	wg.Wait()
	for i, f := range far {
		for j := range g.Nodes {
			if g.Nodes[j].ID != f.nodeID {
				continue
			}
			if res[i].label != "" {
				g.Nodes[j].Label = res[i].label
			}
			g.Nodes[j].Sub, g.Nodes[j].Cpe, g.Nodes[j].Err = res[i].sub, res[i].cpe, res[i].err
			g.Nodes[j].Props = append(g.Nodes[j].Props, res[i].props...)
		}
	}
}

// topoTarget is one compartment the topology covers; Label is what the
// graph shows as the node's compartment ("" never — the base compartment
// is labelled with its own name).
type topoTarget struct{ ID, Label, Path string } // Path: "tenancy (root)/a/b", shown as the Compartment row

// topoCompData is everything one compartment contributes before merging.
type topoCompData struct {
	target     topoTarget
	vcns, drgs []registry.Row
	atts       []registry.Row
	maps       []resourceMapData
	drgTables  []drgTableData
}

// drgTableData is one DRG route table with its raw rules; next-hop
// attachment names are resolved at merge time, once every compartment's
// attachments are known.
type drgTableData struct {
	drgID, id, name string
	ecmp            bool
	rules           []core.DrgRouteRule
}

// fetchTopologyCompartment lists one compartment's network resources,
// reusing the "M" resource map's per-VCN fetch for subnets/route tables/
// gateways.
func fetchTopologyCompartment(ctx context.Context, factory *clients.Factory, scope registry.Scope, t topoTarget) (topoCompData, error) {
	d := topoCompData{target: t}
	scope.CompartmentID = t.ID
	scope.VcnID = ""
	var err error
	if d.vcns, err = fetchAll(ctx, registry.NewVcnResource(factory), scope); err != nil {
		return d, fmt.Errorf("list vcns: %w", err)
	}
	if d.drgs, err = fetchAll(ctx, registry.NewDrgResource(factory), scope); err != nil {
		return d, fmt.Errorf("list drgs: %w", err)
	}
	if d.atts, err = fetchAll(ctx, registry.NewDrgAttachmentResource(factory), scope); err != nil {
		return d, fmt.Errorf("list drg attachments: %w", err)
	}

	for _, drg := range d.drgs {
		rtRows, err := fetchAll(ctx, registry.NewDrgRouteTableResource(factory), registry.Scope{Region: scope.Region, CompartmentID: t.ID, DrgID: drg.ID})
		if err != nil {
			continue // route tables are a detail; the DRG node still shows
		}
		for _, rt := range rtRows {
			rules, _, err := fetchDrgRouteRules(ctx, factory, scope.Region, drg.ID, rt.ID)
			if err != nil {
				continue
			}
			ecmp := false
			if raw, ok := rt.Raw.(core.DrgRouteTable); ok && raw.IsEcmpEnabled != nil {
				ecmp = *raw.IsEcmpEnabled
			}
			d.drgTables = append(d.drgTables, drgTableData{drgID: drg.ID, id: rt.ID, name: rt.Name, ecmp: ecmp, rules: rules})
		}
	}

	d.maps = make([]resourceMapData, len(d.vcns))
	var wg sync.WaitGroup
	for i, row := range d.vcns {
		wg.Add(1)
		go func(i int, row registry.Row) {
			defer wg.Done()
			s := scope
			s.VcnID = row.ID
			// A failed per-VCN fetch leaves that VCN as a bare node rather than failing the whole graph.
			d.maps[i], _ = fetchResourceMapData(ctx, factory, s, row.Name)
		}(i, row)
	}
	wg.Wait()

	return d, nil
}

// fetchTopology gathers the DRG-centred network graph across targets: VCNs
// with their subnets, route tables and gateways, DRGs and every kind of DRG
// attachment, and instances under their subnets. Each node carries its
// compartment so the page can cluster them. The first target is the base —
// its failure fails the whole graph; a descendant's failure just drops it.
func fetchTopology(ctx context.Context, factory *clients.Factory, scope registry.Scope, targets []topoTarget) (topoGraph, error) {
	g := topoGraph{Region: scope.Region}
	results := make([]topoCompData, len(targets))
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t topoTarget) {
			defer wg.Done()
			subtreeSem <- struct{}{}
			defer func() { <-subtreeSem }()
			results[i], errs[i] = fetchTopologyCompartment(ctx, factory, scope, t)
		}(i, t)
	}
	wg.Wait()
	if errs[0] != nil {
		return g, errs[0]
	}

	compPath := map[string]string{}
	for _, t := range targets {
		compPath[t.ID] = t.Path
	}
	var atts []registry.Row
	var drgTables []drgTableData
	attComp := map[string]string{} // attachment row ID -> compartment label
	for i, d := range results {
		if errs[i] != nil {
			continue
		}
		comp := d.target.Label
		for _, row := range d.drgs {
			var props []topoKV
			if raw, ok := row.Raw.(core.Drg); ok {
				props = baseProps(compPath[deref(raw.CompartmentId)], string(raw.LifecycleState), raw.TimeCreated)
			}
			g.addNode(topoNode{ID: row.ID, Label: row.Name, Kind: "drg", Comp: comp, Props: props})
		}
		for j, row := range d.vcns {
			detail := ""
			var props []topoKV
			if v, ok := row.Raw.(core.Vcn); ok {
				detail = deref(v.CidrBlock)
				props = baseProps(compPath[deref(v.CompartmentId)], string(v.LifecycleState), v.TimeCreated)
			}
			g.addNode(topoNode{ID: row.ID, Label: row.Name, Kind: "vcn", Detail: detail, Comp: comp, Props: props})
			m := d.maps[j]
			rtName := map[string]string{}
			for _, rt := range m.routeTables {
				rtName[rt.id] = rt.label
			}
			rtUsers := map[string][]string{}
			var items []string
			for _, sn := range m.subnets {
				line := sn.label + "  " + sn.detail
				if rt := m.subnetToRT[sn.id]; rtName[rt] != "" {
					line += "  · rt: " + rtName[rt]
					rtUsers[rt] = append(rtUsers[rt], sn.label)
				}
				items = append(items, line)
			}
			g.setItems(row.ID, fmt.Sprintf("Subnets (%d)", len(items)), items)
			// Every gateway of the VCN is drawn on it, not just the ones a route rule happens to target.
			for _, c := range m.gateways {
				g.addNode(topoNode{ID: c.id, Label: c.label, Kind: ocidKind(c.id), Detail: c.detail, Comp: comp, Peer: c.peer, PeerStatus: c.status})
				g.addEdge(row.ID, c.id, "gateway", "ok")
			}
			for _, c := range m.connections {
				g.addNode(topoNode{ID: c.id, Label: c.label, Kind: ocidKind(c.id), Detail: c.detail, Comp: comp})
			}
			// Route tables are not nodes: each one is shown on the gateway(s) it routes to
			// (the DRG is covered by its attachment's DRG route table instead).
			for _, rt := range m.routeTables {
				for _, c := range m.rtToConn[rt.id] {
					if ocidKind(c) == "drg" {
						continue
					}
					title := fmt.Sprintf("VCN RT: %s (%d rules) · %s", rt.label, len(m.rtRules[rt.id]), row.Name)
					if users := rtUsers[rt.id]; len(users) > 0 {
						title += " · subnets: " + strings.Join(users, ", ")
					}
					g.addTable(c, topoTable{Title: title, Headers: routeRuleHeaders, Rows: m.rtRules[rt.id], Open: true})
				}
			}
		}
		for _, row := range d.atts {
			attComp[row.ID] = comp
		}
		atts = append(atts, d.atts...)
		drgTables = append(drgTables, d.drgTables...)
	}

	// Local peering: a line between each pair of LPGs that are peered (the peer may be in a compartment
	// outside the graph, in which case there is nothing to link to).
	for _, n := range g.Nodes {
		if n.Kind != "lpg" || n.Peer == "" || !g.hasNode(n.Peer) || g.hasEdge(n.ID, n.Peer) || g.hasEdge(n.Peer, n.ID) {
			continue
		}
		state := "ok"
		if n.PeerStatus != "" && n.PeerStatus != "PEERED" {
			state = "down"
		}
		g.addNamedEdge(n.ID, n.Peer, "peering", state, n.Label+" ↔ "+g.labelOf(n.Peer), "")
	}

	// Attachments last: a VCN or DRG may live in a different compartment than the attachment.
	farNode := map[string]string{} // attachment row ID -> connection node ID
	seenFar := map[string]bool{}
	var far []farRef
	for _, row := range atts {
		a, ok := row.Raw.(registry.DrgAttachmentRow)
		if !ok {
			continue
		}
		state := "ok"
		if a.LifecycleState != core.DrgAttachmentLifecycleStateAttached {
			state = "down"
		}
		drgID := deref(a.DrgId)
		if _, isVcn := a.NetworkDetails.(core.VcnDrgAttachmentNetworkDetails); isVcn || a.NetworkDetails == nil {
			vcnID := deref(a.VcnId)
			if nd, ok := a.NetworkDetails.(core.VcnDrgAttachmentNetworkDetails); ok {
				vcnID = deref(nd.Id)
			}
			if g.hasNode(vcnID) && g.hasNode(drgID) {
				g.addNamedEdge(vcnID, drgID, "attached", state, row.Name, row.ID)
			}
			continue
		}
		kind, rel, nodeID := attachmentNode(a.NetworkDetails, row.ID)
		if kind == "" || !g.hasNode(drgID) {
			continue
		}
		label := a.TargetName
		if label == "" {
			label = row.Name
		}
		g.addNode(topoNode{ID: nodeID, Label: label, Kind: kind, Detail: row.Name, Comp: g.compOf(drgID)})
		g.addNamedEdge(drgID, nodeID, rel, state, row.Name, row.ID)
		farNode[row.ID] = nodeID
		if !seenFar[nodeID] {
			seenFar[nodeID] = true
			far = append(far, farRef{nodeID: nodeID, kind: kind, nd: a.NetworkDetails})
		}
	}
	g.describeConnections(ctx, factory, scope.Region, far, compPath)

	g.addDrgTables(atts, drgTables, farNode)
	g.addEdgeRoutes(atts, drgTables)
	g.addEdgeProps(ctx, factory, scope.Region, atts, drgTables, compPath)

	return g, nil
}

// renderTopologyHTML bakes the graph into one self-contained HTML file.
func renderTopologyHTML(g topoGraph) (string, error) {
	// json.Marshal escapes <, > and & so the data can't close the <script> early.
	dataJSON, err := json.Marshal(g)
	if err != nil {
		return "", err
	}
	return strings.Replace(topologyTemplate, "__TOCI_DATA__", string(dataJSON), 1), nil
}

func openInBrowser(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	_ = cmd.Start() // best effort: the file path is also shown in the status line
}

// buildTopology is the "T" key's tea.Cmd: fetch, write the HTML next to
// the other exports, and open it.
func (m Model) buildTopology() tea.Cmd {
	factory, scope := m.factory, m.scope
	baseName := scope.CompartmentID
	if n := m.compTree.node(scope.CompartmentID); n != nil {
		baseName = n.Name
	}
	path := func(id string) string {
		crumbs := m.compTree.pathTo(id)
		if len(crumbs) == 0 {
			return ""
		}
		crumbs[0].Name += " (root)"
		return breadcrumbLabel(crumbs)
	}
	targets := []topoTarget{{ID: scope.CompartmentID, Label: baseName, Path: path(scope.CompartmentID)}}
	if base := m.compTree.node(scope.CompartmentID); m.subtreeOn && base != nil {
		for _, d := range base.descendants() {
			targets = append(targets, topoTarget{ID: d.ID, Label: baseName + "/" + m.compTree.relativePath(base.ID, d.ID), Path: path(d.ID)})
		}
	}
	return func() tea.Msg {
		g, err := fetchTopology(context.Background(), factory, scope, targets)
		if err != nil {
			return topologyMsg{err: err}
		}
		content, err := renderTopologyHTML(g)
		if err != nil {
			return topologyMsg{err: err}
		}
		path := fmt.Sprintf("toci-topology-%s-%s.html", filenameUnsafe.ReplaceAllString(scope.Region, "-"), time.Now().Format("20060102-150405"))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return topologyMsg{err: err}
		}
		openInBrowser(path)
		failed := 0
		for _, n := range g.Nodes {
			if n.Err != "" {
				failed++
			}
		}
		return topologyMsg{path: path, failed: failed}
	}
}
