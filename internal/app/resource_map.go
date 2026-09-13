package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/clients"
	"toci/internal/registry"
)

type resourceMapMsg struct {
	data resourceMapData
	err  error
}

// resourceMapData is everything renderResourceMap needs, already resolved
// to display labels — fetchResourceMapData is the only thing that talks
// to OCI.
type resourceMapData struct {
	vcnName, vcnCidr string
	subnets          []mapNode
	routeTables      []mapNode
	connections      []mapNode

	subnetToRT map[string]string   // subnet ID -> route table ID
	rtToConn   map[string][]string // route table ID -> connection entity IDs, in first-seen order
}

// buildResourceMap fetches and renders the AWS-console-style resource map
// (VCN / Subnets / Route Tables / Network Connections, with connector
// lines) for the currently VCN-filtered scope — the "M" key's view,
// alongside "m"'s Mermaid file export.
func (m Model) buildResourceMap() tea.Cmd {
	factory := m.factory
	scope := m.scope
	vcnName := m.vcnFilterName
	return func() tea.Msg {
		data, err := fetchResourceMapData(context.Background(), factory, scope, vcnName)
		if err != nil {
			return resourceMapMsg{err: err}
		}
		return resourceMapMsg{data: data}
	}
}

// gatewayEntity is a resource map "network connection" box's content
// before it's placed under a route table — name plus an optional detail
// line (currently just a NAT gateway's public IP; other gateway kinds
// have no single-field equivalent worth showing here).
type gatewayEntity struct{ name, detail string }

// listGatewayNames is the shared shape behind every gateway type's
// "resolve OCID -> gatewayEntity for this VCN" lookup below — OCI has no
// single "list network connections" endpoint, just one List call per
// gateway kind, each with an identical (paginate, filter by VcnId) shape.
func listGatewayNames(ctx context.Context, compartmentID, vcnID string, list func(page string) (ids []string, entities []gatewayEntity, next string, err error)) (map[string]gatewayEntity, error) {
	out := map[string]gatewayEntity{}
	page := ""
	for {
		ids, entities, next, err := list(page)
		if err != nil {
			return nil, err
		}
		for i, id := range ids {
			out[id] = entities[i]
		}
		if next == "" {
			return out, nil
		}
		page = next
	}
}

// fetchResourceMapData resolves subnets, the route tables they actually
// use, and the gateways/DRGs those route tables' rules actually target —
// only what's reachable from this VCN's subnets, matching the AWS console
// resource map's own framing rather than "everything that technically
// exists in this VCN."
func fetchResourceMapData(ctx context.Context, factory *clients.Factory, scope registry.Scope, vcnName string) (resourceMapData, error) {
	data := resourceMapData{
		vcnName:    vcnName,
		subnetToRT: map[string]string{},
		rtToConn:   map[string][]string{},
	}

	vnClient, err := factory.VirtualNetwork(scope.Region)
	if err != nil {
		return data, err
	}

	subnetRows, err := fetchAll(ctx, registry.NewSubnetResource(factory), scope)
	if err != nil {
		return data, fmt.Errorf("list subnets: %w", err)
	}
	hasAD := false
	for _, row := range subnetRows {
		sn, _ := row.Raw.(core.Subnet)
		ad := deref(sn.AvailabilityDomain)
		if ad != "" {
			hasAD = true
		}
		data.subnets = append(data.subnets, mapNode{id: row.ID, label: row.Name, detail: deref(sn.CidrBlock), group: ad})
		data.subnetToRT[row.ID] = deref(sn.RouteTableId)
	}
	if !hasAD {
		// The overwhelmingly common case (OCI's default is a regional
		// subnet, with no AvailabilityDomain set at all) — an AD group
		// header on every single subnet would be redundant noise, so only
		// group when at least one subnet is actually AD-specific.
		for i := range data.subnets {
			data.subnets[i].group = ""
		}
	}

	rtRows, err := fetchAll(ctx, registry.NewRouteTableResource(factory), scope)
	if err != nil {
		return data, fmt.Errorf("list route tables: %w", err)
	}
	usedRT := map[string]bool{}
	for _, id := range data.subnetToRT {
		usedRT[id] = true
	}
	rtByID := make(map[string]core.RouteTable, len(rtRows))
	for _, row := range rtRows {
		rt, _ := row.Raw.(core.RouteTable)
		rtByID[row.ID] = rt
		if usedRT[row.ID] {
			data.routeTables = append(data.routeTables, mapNode{id: row.ID, label: row.Name})
		}
	}

	entities, err := fetchGatewayNames(ctx, vnClient, scope.CompartmentID, scope.VcnID)
	if err != nil {
		return data, fmt.Errorf("list gateways: %w", err)
	}
	if drgIDs, err := attachedDrgIDs(ctx, vnClient, scope.CompartmentID, scope.VcnID); err == nil && len(drgIDs) > 0 {
		if drgRows, err := fetchAll(ctx, registry.NewDrgResource(factory), registry.Scope{Region: scope.Region, CompartmentID: scope.CompartmentID}); err == nil {
			for _, row := range drgRows {
				if drgIDs[row.ID] {
					entities[row.ID] = gatewayEntity{name: row.Name}
				}
			}
		}
	}

	connSeen := map[string]bool{}
	for _, rt := range data.routeTables {
		full := rtByID[rt.id]
		for _, r := range full.RouteRules {
			eid := deref(r.NetworkEntityId)
			entity, ok := entities[eid]
			if !ok {
				continue // not a gateway/DRG this map tracks (e.g. an instance/private-IP route) — leave it out
			}
			if !slices.Contains(data.rtToConn[rt.id], eid) {
				data.rtToConn[rt.id] = append(data.rtToConn[rt.id], eid)
			}
			if !connSeen[eid] {
				connSeen[eid] = true
				data.connections = append(data.connections, mapNode{id: eid, label: entity.name, detail: entity.detail})
			}
		}
	}

	if vcnRows, err := fetchAll(ctx, registry.NewVcnResource(factory), registry.Scope{Region: scope.Region, CompartmentID: scope.CompartmentID}); err == nil {
		for _, row := range vcnRows {
			if row.ID == scope.VcnID {
				if v, ok := row.Raw.(core.Vcn); ok {
					data.vcnCidr = deref(v.CidrBlock)
				}
				break
			}
		}
	}

	return data, nil
}

// fetchGatewayNames resolves every Internet/NAT/Service/Local Peering
// Gateway attached to this VCN to a gatewayEntity — the "network
// connections" a route rule's NetworkEntityId can point at, besides a DRG
// (handled separately by the caller via attachedDrgIDs, the same helper
// the Mermaid diagram export already uses).
func fetchGatewayNames(ctx context.Context, vnClient core.VirtualNetworkClient, compartmentID, vcnID string) (map[string]gatewayEntity, error) {
	out := map[string]gatewayEntity{}

	merge := func(m map[string]gatewayEntity, err error) error {
		if err != nil {
			return err
		}
		for id, e := range m {
			out[id] = e
		}
		return nil
	}

	if err := merge(listGatewayNames(ctx, compartmentID, vcnID, func(page string) ([]string, []gatewayEntity, string, error) {
		req := core.ListInternetGatewaysRequest{CompartmentId: &compartmentID, VcnId: &vcnID}
		if page != "" {
			req.Page = &page
		}
		resp, err := vnClient.ListInternetGateways(ctx, req)
		if err != nil {
			return nil, nil, "", err
		}
		ids, entities := make([]string, len(resp.Items)), make([]gatewayEntity, len(resp.Items))
		for i, g := range resp.Items {
			ids[i], entities[i] = deref(g.Id), gatewayEntity{name: deref(g.DisplayName)}
		}
		return ids, entities, deref(resp.OpcNextPage), nil
	})); err != nil {
		return nil, err
	}

	if err := merge(listGatewayNames(ctx, compartmentID, vcnID, func(page string) ([]string, []gatewayEntity, string, error) {
		req := core.ListNatGatewaysRequest{CompartmentId: &compartmentID, VcnId: &vcnID}
		if page != "" {
			req.Page = &page
		}
		resp, err := vnClient.ListNatGateways(ctx, req)
		if err != nil {
			return nil, nil, "", err
		}
		ids, entities := make([]string, len(resp.Items)), make([]gatewayEntity, len(resp.Items))
		for i, g := range resp.Items {
			ids[i] = deref(g.Id)
			entities[i] = gatewayEntity{name: deref(g.DisplayName), detail: deref(g.NatIp)}
		}
		return ids, entities, deref(resp.OpcNextPage), nil
	})); err != nil {
		return nil, err
	}

	if err := merge(listGatewayNames(ctx, compartmentID, vcnID, func(page string) ([]string, []gatewayEntity, string, error) {
		req := core.ListServiceGatewaysRequest{CompartmentId: &compartmentID, VcnId: &vcnID}
		if page != "" {
			req.Page = &page
		}
		resp, err := vnClient.ListServiceGateways(ctx, req)
		if err != nil {
			return nil, nil, "", err
		}
		ids, entities := make([]string, len(resp.Items)), make([]gatewayEntity, len(resp.Items))
		for i, g := range resp.Items {
			ids[i], entities[i] = deref(g.Id), gatewayEntity{name: deref(g.DisplayName)}
		}
		return ids, entities, deref(resp.OpcNextPage), nil
	})); err != nil {
		return nil, err
	}

	if err := merge(listGatewayNames(ctx, compartmentID, vcnID, func(page string) ([]string, []gatewayEntity, string, error) {
		req := core.ListLocalPeeringGatewaysRequest{CompartmentId: &compartmentID, VcnId: &vcnID}
		if page != "" {
			req.Page = &page
		}
		resp, err := vnClient.ListLocalPeeringGateways(ctx, req)
		if err != nil {
			return nil, nil, "", err
		}
		ids, entities := make([]string, len(resp.Items)), make([]gatewayEntity, len(resp.Items))
		for i, g := range resp.Items {
			ids[i], entities[i] = deref(g.Id), gatewayEntity{name: deref(g.DisplayName)}
		}
		return ids, entities, deref(resp.OpcNextPage), nil
	})); err != nil {
		return nil, err
	}

	return out, nil
}

// pathHighlight is which boxes and edges renderResourceMap should draw in
// the accent color for a selected subnet — its own box, the route table
// it uses, and every gateway/DRG that route table's rules target, plus
// the specific connector edges linking them (see resourceMapPath).
type pathHighlight struct {
	boxes  map[string]bool
	edgesA []connectorEdge // subnet -> route table
	edgesB []connectorEdge // route table -> connections
}

// resourceMapPath resolves the highlighted path for data.subnets[selected]
// — its route table and that route table's own gateway/DRG targets — into
// row-based connectorEdges via the already-computed row maps. selected
// outside [0, len(subnets)) (including the "no subnets" sentinel -1)
// yields a zero-value pathHighlight, i.e. nothing highlighted.
func resourceMapPath(data resourceMapData, selected int, subnetRow, rtRow, connRow map[string]int) pathHighlight {
	if selected < 0 || selected >= len(data.subnets) {
		return pathHighlight{}
	}
	subnetID := data.subnets[selected].id
	rtID := data.subnetToRT[subnetID]

	h := pathHighlight{boxes: map[string]bool{subnetID: true}}
	if rtID == "" {
		return h
	}
	h.boxes[rtID] = true
	if sr, ok := subnetRow[subnetID]; ok {
		if rr, ok := rtRow[rtID]; ok {
			h.edgesA = []connectorEdge{{from: sr, to: rr}}
		}
	}
	for _, connID := range data.rtToConn[rtID] {
		h.boxes[connID] = true
		if rr, ok := rtRow[rtID]; ok {
			if cr, ok := connRow[connID]; ok {
				h.edgesB = append(h.edgesB, connectorEdge{from: rr, to: cr})
			}
		}
	}
	return h
}

// renderResourceMap composes the VCN / Subnets / Route Tables / Network
// Connections columns and their connector buses into the full resource
// map view (shown in modeDetail via the "M" key). selected is the index
// into data.subnets currently picked (j/k in modeDetail move it — see
// updateDetail), highlighting its path through to the gateway(s) it
// reaches; -1 highlights nothing.
func renderResourceMap(data resourceMapData, selected int) string {
	vcnW := max(len("VCN"), len([]rune(data.vcnName)), len([]rune(data.vcnCidr)), 20)
	subnetW := mapColWidth(data.subnets, 30)
	rtW := mapColWidth(data.routeTables, 26)
	connW := mapColWidth(data.connections, 26)

	// A first pass to learn row positions before we know what to
	// highlight — row assignment doesn't depend on highlighting, so this
	// pass's rendered lines are simply discarded once resourceMapPath has
	// resolved the highlighted boxes/edges from them.
	_, subnetRow := renderMapColumn(data.subnets, subnetW, nil)
	_, rtRow := renderMapColumn(data.routeTables, rtW, nil)
	_, connRow := renderMapColumn(data.connections, connW, nil)
	path := resourceMapPath(data, selected, subnetRow, rtRow, connRow)

	subnetLines, _ := renderMapColumn(data.subnets, subnetW, path.boxes)
	rtLines, _ := renderMapColumn(data.routeTables, rtW, path.boxes)
	connLines, _ := renderMapColumn(data.connections, connW, path.boxes)

	height := max(len(subnetLines), len(rtLines), len(connLines))
	subnetLines = padLines(subnetLines, height)
	rtLines = padLines(rtLines, height)
	connLines = padLines(connLines, height)

	subnetToRT := map[string][]string{}
	for sid, rtID := range data.subnetToRT {
		if rtID != "" {
			subnetToRT[sid] = []string{rtID}
		}
	}
	busA := applyBusHighlight(renderConnectorBus(buildEdges(subnetRow, subnetToRT, rtRow), height), path.edgesA)
	busB := applyBusHighlight(renderConnectorBus(buildEdges(rtRow, data.rtToConn, connRow), height), path.edgesB)

	vcnLabel := data.vcnName
	if data.vcnCidr != "" {
		// Never highlighted, so no nesting hazard pre-styling this (see
		// renderMapColumn's own comment on why that's not safe to do
		// unconditionally for a box that might be).
		vcnLabel += "\n" + stateTextWarn.Render(data.vcnCidr)
	}
	vcnBox := strings.Split(mapBoxStyle(vcnW, false).Render(vcnLabel), "\n")

	block := func(title string, count int, lines []string) string {
		return strings.Join(append(prefixTitle(title, count), lines...), "\n")
	}
	busBlock := func(lines []string) string {
		return strings.Join(append(busBlankPrefix(), lines...), "\n")
	}

	return lipgloss.JoinHorizontal(lipgloss.Top,
		block("VCN", -1, vcnBox),
		"  ",
		block("Subnets", len(data.subnets), subnetLines),
		busBlock(busA),
		block("Route Tables", len(data.routeTables), rtLines),
		busBlock(busB),
		block("Network Connections", len(data.connections), connLines),
	)
}
