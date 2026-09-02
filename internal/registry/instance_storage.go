package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// fetchInstanceStorage resolves each instance's total attached storage in
// GB — its boot volume plus any block volumes attached later, whichever
// instance the OCID maps to (OCI itself has no single "total storage"
// field on Instance).
//
// There's no per-instance shortcut for any of this, but every list API
// involved is scoped to (compartment, availability domain) rather than to
// a single instance/volume — so this calls each one once per distinct AD
// among the given instances (typically 1-3 ADs in a region), not once per
// instance: ListBootVolumeAttachments/ListVolumeAttachments (on the
// Compute client) map instanceID -> volumeIDs, then
// ListBootVolumes/ListVolumes (on the Blockstorage client) give the size of
// every volume in that AD. A failed call for one AD just leaves that AD's
// instances without a storage value rather than failing the whole listing.
func fetchInstanceStorage(ctx context.Context, computeClient core.ComputeClient, bsClient core.BlockstorageClient, compartmentID string, availabilityDomains []string) map[string]int64 {
	instanceToVolumes := make(map[string][]string)
	volumeSize := make(map[string]int64)

	seenAD := make(map[string]bool)
	for _, ad := range availabilityDomains {
		if ad == "" || seenAD[ad] {
			continue
		}
		seenAD[ad] = true
		ad := ad

		page := ""
		for {
			req := core.ListBootVolumeAttachmentsRequest{AvailabilityDomain: &ad, CompartmentId: &compartmentID}
			if page != "" {
				req.Page = &page
			}
			resp, err := computeClient.ListBootVolumeAttachments(ctx, req)
			if err != nil {
				break
			}
			for _, a := range resp.Items {
				if a.InstanceId == nil || a.BootVolumeId == nil {
					continue
				}
				instanceToVolumes[*a.InstanceId] = append(instanceToVolumes[*a.InstanceId], *a.BootVolumeId)
			}
			if resp.OpcNextPage == nil {
				break
			}
			page = *resp.OpcNextPage
		}

		page = ""
		for {
			req := core.ListBootVolumesRequest{AvailabilityDomain: &ad, CompartmentId: &compartmentID}
			if page != "" {
				req.Page = &page
			}
			resp, err := bsClient.ListBootVolumes(ctx, req)
			if err != nil {
				break
			}
			for _, v := range resp.Items {
				if v.Id == nil || v.SizeInGBs == nil {
					continue
				}
				volumeSize[*v.Id] = *v.SizeInGBs
			}
			if resp.OpcNextPage == nil {
				break
			}
			page = *resp.OpcNextPage
		}

		page = ""
		for {
			req := core.ListVolumeAttachmentsRequest{AvailabilityDomain: &ad, CompartmentId: &compartmentID}
			if page != "" {
				req.Page = &page
			}
			resp, err := computeClient.ListVolumeAttachments(ctx, req)
			if err != nil {
				break
			}
			for _, a := range resp.Items {
				if a.GetLifecycleState() != core.VolumeAttachmentLifecycleStateAttached {
					continue
				}
				instID, volID := a.GetInstanceId(), a.GetVolumeId()
				if instID == nil || volID == nil {
					continue
				}
				instanceToVolumes[*instID] = append(instanceToVolumes[*instID], *volID)
			}
			if resp.OpcNextPage == nil {
				break
			}
			page = *resp.OpcNextPage
		}

		page = ""
		for {
			req := core.ListVolumesRequest{AvailabilityDomain: &ad, CompartmentId: &compartmentID}
			if page != "" {
				req.Page = &page
			}
			resp, err := bsClient.ListVolumes(ctx, req)
			if err != nil {
				break
			}
			for _, v := range resp.Items {
				if v.Id == nil || v.SizeInGBs == nil {
					continue
				}
				volumeSize[*v.Id] = *v.SizeInGBs
			}
			if resp.OpcNextPage == nil {
				break
			}
			page = *resp.OpcNextPage
		}
	}

	return sumInstanceVolumeSizes(instanceToVolumes, volumeSize)
}

// sumInstanceVolumeSizes totals, per instance, the sizes of every volume
// (boot + block) attached to it — given instanceID->volumeIDs and
// volumeID->sizeInGBs — split out from fetchInstanceStorage so this join/sum
// logic has a test that doesn't need a live SDK client. A volume whose size
// wasn't resolved (e.g. that AD's ListVolumes call failed) is skipped
// rather than counted as zero, so a partial failure undercounts instead of
// silently reporting a wrong total as if it were complete.
func sumInstanceVolumeSizes(instanceToVolumes map[string][]string, volumeSize map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(instanceToVolumes))
	for instanceID, volumeIDs := range instanceToVolumes {
		var total int64
		var found bool
		for _, volID := range volumeIDs {
			if size, ok := volumeSize[volID]; ok {
				total += size
				found = true
			}
		}
		if found {
			out[instanceID] = total
		}
	}
	return out
}
