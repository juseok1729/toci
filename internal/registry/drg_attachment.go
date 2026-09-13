package registry

import (
	"context"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

type DrgAttachmentResource struct {
	factory *clients.Factory
}

func NewDrgAttachmentResource(f *clients.Factory) *DrgAttachmentResource {
	return &DrgAttachmentResource{factory: f}
}

func (r *DrgAttachmentResource) Key() string   { return "drg-attachment" }
func (r *DrgAttachmentResource) Label() string { return "DRG Attachments" }

// DrgAttachmentRow wraps core.DrgAttachment with TargetName — the resolved
// display name of whatever NetworkDetails points at (see
// resolveDrgAttachmentTargetName), so the ATTACHED TO column can show a
// name instead of a bare OCID.
type DrgAttachmentRow struct {
	core.DrgAttachment
	TargetName string
}

func (r *DrgAttachmentResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(DrgAttachmentRow).DisplayName)
		}},
		{Header: "TYPE", Width: 16, Get: func(row Row) string {
			return drgAttachmentType(row.Raw.(DrgAttachmentRow).NetworkDetails)
		}},
		{Header: "ATTACHED TO", Width: 60, Get: func(row Row) string {
			a := row.Raw.(DrgAttachmentRow)
			if a.TargetName != "" {
				return a.TargetName
			}
			// Resolution failed (deleted target, missing permission, an
			// attachment kind with no name to resolve) — the OCID is still
			// more useful than a blank cell.
			return drgAttachmentTargetID(a.DrgAttachment)
		}},
	}
}

func (r *DrgAttachmentResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListDrgAttachmentsRequest{CompartmentId: &s.CompartmentID}
	if s.DrgID != "" {
		req.DrgId = &s.DrgID
	}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListDrgAttachments(ctx, req)
	if err != nil {
		return nil, "", err
	}

	// Resolving the attached resource's name is one extra Get call per row
	// (GetVcn/GetVirtualCircuit/GetRemotePeeringConnection/
	// GetIPSecConnectionTunnel) — fanned out like DB System/Instance/Exadata
	// already do, writing only to each goroutine's own rows[i].
	rows := make([]Row, len(resp.Items))
	var wg sync.WaitGroup
	for i, a := range resp.Items {
		wg.Add(1)
		go func(i int, a core.DrgAttachment) {
			defer wg.Done()
			rows[i] = Row{ID: deref(a.Id), Name: deref(a.DisplayName), TimeCreated: timeOf(a.TimeCreated), Raw: DrgAttachmentRow{
				DrgAttachment: a,
				TargetName:    resolveDrgAttachmentTargetName(ctx, client, a),
			}}
		}(i, a)
	}
	wg.Wait()

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}

// resolveDrgAttachmentTargetName looks up the display name of whatever's on
// the other end of a DRG attachment. Each NetworkDetails kind names its
// target through a different resource type (and a different Get call) —
// Loopback attachments and old attachments predating NetworkDetails (bare
// VcnId) have no NetworkDetails at all, so those fall back to VcnId. Any
// lookup error (deleted target, no permission) yields "", letting the
// caller fall back to the OCID rather than fail the whole row.
func resolveDrgAttachmentTargetName(ctx context.Context, client core.VirtualNetworkClient, a core.DrgAttachment) string {
	switch nd := a.NetworkDetails.(type) {
	case core.VcnDrgAttachmentNetworkDetails:
		return vcnDisplayName(ctx, client, nd.Id)
	case core.VirtualCircuitDrgAttachmentNetworkDetails:
		if nd.Id == nil {
			return ""
		}
		resp, err := client.GetVirtualCircuit(ctx, core.GetVirtualCircuitRequest{VirtualCircuitId: nd.Id})
		if err != nil {
			return ""
		}
		return deref(resp.DisplayName)
	case core.RemotePeeringConnectionDrgAttachmentNetworkDetails:
		if nd.Id == nil {
			return ""
		}
		resp, err := client.GetRemotePeeringConnection(ctx, core.GetRemotePeeringConnectionRequest{RemotePeeringConnectionId: nd.Id})
		if err != nil {
			return ""
		}
		return deref(resp.DisplayName)
	case core.IpsecTunnelDrgAttachmentNetworkDetails:
		if nd.Id == nil || nd.IpsecConnectionId == nil {
			return ""
		}
		resp, err := client.GetIPSecConnectionTunnel(ctx, core.GetIPSecConnectionTunnelRequest{IpscId: nd.IpsecConnectionId, TunnelId: nd.Id})
		if err != nil {
			return ""
		}
		// Unlike a VCN/VC/RPC, a tunnel's own DisplayName is genuinely
		// optional and commonly left blank by the console — fall back to
		// the parent IPSec connection's name, still more useful than the
		// raw tunnel OCID.
		if name := deref(resp.DisplayName); name != "" {
			return name
		}
		connResp, err := client.GetIPSecConnection(ctx, core.GetIPSecConnectionRequest{IpscId: nd.IpsecConnectionId})
		if err != nil {
			return ""
		}
		return deref(connResp.DisplayName)
	default:
		return vcnDisplayName(ctx, client, a.VcnId)
	}
}

func vcnDisplayName(ctx context.Context, client core.VirtualNetworkClient, vcnID *string) string {
	if vcnID == nil {
		return ""
	}
	resp, err := client.GetVcn(ctx, core.GetVcnRequest{VcnId: vcnID})
	if err != nil {
		return ""
	}
	return deref(resp.DisplayName)
}

// drgAttachmentType renders a DRG attachment's NetworkDetails as a short
// human label. The SDK interface only exposes GetId() — the concrete type
// (which resource kind is on the other end) has to come from a type switch.
func drgAttachmentType(nd core.DrgAttachmentNetworkDetails) string {
	switch nd.(type) {
	case core.VcnDrgAttachmentNetworkDetails:
		return "VCN"
	case core.IpsecTunnelDrgAttachmentNetworkDetails:
		return "IPSec Tunnel"
	case core.VirtualCircuitDrgAttachmentNetworkDetails:
		return "Virtual Circuit"
	case core.RemotePeeringConnectionDrgAttachmentNetworkDetails:
		return "Remote Peering"
	case core.LoopBackDrgAttachmentNetworkDetails:
		return "Loopback"
	default:
		return "-"
	}
}

// drgAttachmentTargetID returns the OCID of whatever's attached (a VCN,
// IPSec tunnel, ...) via NetworkDetails.GetId(), falling back to the
// deprecated top-level VcnId field for older VCN attachments predating
// NetworkDetails.
func drgAttachmentTargetID(a core.DrgAttachment) string {
	if a.NetworkDetails != nil {
		if id := a.NetworkDetails.GetId(); id != nil {
			return *id
		}
	}
	return deref(a.VcnId)
}
