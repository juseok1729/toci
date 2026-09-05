package registry

import (
	"context"

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

func (r *DrgAttachmentResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.DrgAttachment).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.DrgAttachment).LifecycleState)
		}},
		{Header: "TYPE", Width: 16, Get: func(row Row) string {
			return drgAttachmentType(row.Raw.(core.DrgAttachment).NetworkDetails)
		}},
		{Header: "ATTACHED TO", Width: 60, Get: func(row Row) string {
			return drgAttachmentTargetID(row.Raw.(core.DrgAttachment))
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

	rows := make([]Row, 0, len(resp.Items))
	for _, a := range resp.Items {
		rows = append(rows, Row{ID: deref(a.Id), Name: deref(a.DisplayName), TimeCreated: timeOf(a.TimeCreated), Raw: a})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
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
