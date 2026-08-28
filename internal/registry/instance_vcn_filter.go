package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// vcnSubnetIDs returns the OCIDs of every subnet in the given VCN. Shared by
// any resource whose own List API can't filter by VcnId directly and has to
// join against subnet membership instead (Instance, LoadBalancer).
func vcnSubnetIDs(ctx context.Context, vnClient core.VirtualNetworkClient, compartmentID, vcnID string) (map[string]bool, error) {
	subnetIDs := make(map[string]bool)
	page := ""
	for {
		req := core.ListSubnetsRequest{CompartmentId: &compartmentID, VcnId: &vcnID}
		if page != "" {
			req.Page = &page
		}
		resp, err := vnClient.ListSubnets(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, sn := range resp.Items {
			subnetIDs[deref(sn.Id)] = true
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return subnetIDs, nil
}

// InstanceSubnetIDs returns instanceID -> subnetID for every instance in
// the compartment with at least one VNIC attachment (first attachment
// wins for multi-VNIC instances — good enough for grouping purposes, and
// avoids the extra GetVnic-per-instance cost that pinning down the true
// primary VNIC would need — see instance_ip.go for that heavier path).
// Exported for the topology diagram builder (diagram.go), which needs to
// know exactly which subnet each instance sits in, not just VCN
// membership.
func InstanceSubnetIDs(ctx context.Context, computeClient core.ComputeClient, compartmentID string) (map[string]string, error) {
	out := make(map[string]string)
	page := ""
	for {
		req := core.ListVnicAttachmentsRequest{CompartmentId: &compartmentID}
		if page != "" {
			req.Page = &page
		}
		resp, err := computeClient.ListVnicAttachments(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, va := range resp.Items {
			if va.SubnetId == nil || va.InstanceId == nil {
				continue
			}
			if _, ok := out[*va.InstanceId]; !ok {
				out[*va.InstanceId] = *va.SubnetId
			}
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return out, nil
}

// instanceIDsInVcn returns the OCIDs of instances with a VNIC in the given
// VCN. Instances don't carry a VCN reference themselves — only their VNIC
// attachments do, via subnet — so this joins the VCN's subnets against
// every instance's subnet instead of one lookup per instance.
func instanceIDsInVcn(ctx context.Context, vnClient core.VirtualNetworkClient, computeClient core.ComputeClient, compartmentID, vcnID string) (map[string]bool, error) {
	subnetIDs, err := vcnSubnetIDs(ctx, vnClient, compartmentID, vcnID)
	if err != nil {
		return nil, err
	}
	instSubnets, err := InstanceSubnetIDs(ctx, computeClient, compartmentID)
	if err != nil {
		return nil, err
	}

	instanceIDs := make(map[string]bool, len(instSubnets))
	for instanceID, subnetID := range instSubnets {
		if subnetIDs[subnetID] {
			instanceIDs[instanceID] = true
		}
	}
	return instanceIDs, nil
}
