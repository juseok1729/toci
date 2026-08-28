package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
)

type instanceIPs struct {
	Public  string
	Private string
}

// fetchInstanceIPs resolves each instance's public/private IP. There's no
// bulk "list VNICs with their IPs" endpoint, so this walks every VNIC
// attachment in the compartment once (ListVnicAttachments, paginated) and
// then calls GetVnic per attached VNIC to read its IPs and IsPrimary flag —
// one GetVnic per instance in the common single-NIC case, more only for
// instances with multiple VNICs (where picking the right one needs the
// IsPrimary flag, which only GetVnic exposes). A failed GetVnic call just
// leaves that instance's IPs blank rather than failing the whole listing.
func fetchInstanceIPs(ctx context.Context, computeClient core.ComputeClient, vnClient core.VirtualNetworkClient, compartmentID string) map[string]instanceIPs {
	byInstance := make(map[string][]string) // instanceID -> attached VNIC IDs
	page := ""
	for {
		req := core.ListVnicAttachmentsRequest{CompartmentId: &compartmentID}
		if page != "" {
			req.Page = &page
		}
		resp, err := computeClient.ListVnicAttachments(ctx, req)
		if err != nil {
			return nil
		}
		for _, va := range resp.Items {
			if va.LifecycleState != core.VnicAttachmentLifecycleStateAttached || va.VnicId == nil || va.InstanceId == nil {
				continue
			}
			byInstance[*va.InstanceId] = append(byInstance[*va.InstanceId], *va.VnicId)
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}

	out := make(map[string]instanceIPs, len(byInstance))
	for instanceID, vnicIDs := range byInstance {
		var fallback *instanceIPs
		for _, vnicID := range vnicIDs {
			vnicID := vnicID
			resp, err := vnClient.GetVnic(ctx, core.GetVnicRequest{VnicId: &vnicID})
			if err != nil {
				continue
			}
			ips := instanceIPs{Public: deref(resp.PublicIp), Private: deref(resp.PrivateIp)}
			if resp.IsPrimary != nil && *resp.IsPrimary {
				out[instanceID] = ips
				fallback = nil
				break
			}
			if fallback == nil {
				fallback = &ips
			}
		}
		if fallback != nil {
			out[instanceID] = *fallback
		}
	}
	return out
}
