package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// instanceStorage is an instance's attached storage, split the way the
// Instance table shows it: boot volume size, and the summed size of every
// block volume attached (an instance can have several) — kept separate
// since they answer different questions ("how big is the OS disk" vs "how
// much data storage is attached").
type instanceStorage struct {
	BootGB  *int64
	BlockGB *int64
}

// maxSaneVolumeGB is a sanity ceiling on a single volume's reported size.
// Real OCI Boot/Block Volumes top out at 32TB (32768 GB); a File Storage
// (NFS) mount reports its size in the exabyte range (OCI's placeholder for
// "elastic, no fixed capacity"). ListVolumes/ListBootVolumes should only
// ever return real block/boot volumes, but if a mount like that ever shows
// up in the response, summing its size in would make an instance's total
// storage read as "8 exabytes" instead of its actual disk size — so
// anything this far beyond OCI's real ceiling is excluded rather than
// trusted. Set an order of magnitude above the real ceiling for headroom
// against OCI raising it, while staying far below exabyte-scale junk.
const maxSaneVolumeGB = 1 << 20 // 1 PB

// sanitizeVolumeSize returns size if it's within maxSaneVolumeGB, or nil
// otherwise (a nil input included) — split out from fetchInstanceStorage
// so this filter has a test that doesn't need a live SDK client.
func sanitizeVolumeSize(size *int64) *int64 {
	if size == nil || *size > maxSaneVolumeGB {
		return nil
	}
	return size
}

// fetchInstanceStorage resolves each instance's boot and (summed) block
// volume sizes in GB — OCI itself has no single "storage" field on
// Instance.
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
func fetchInstanceStorage(ctx context.Context, computeClient core.ComputeClient, bsClient core.BlockstorageClient, compartmentID string, availabilityDomains []string) map[string]instanceStorage {
	instanceToBoot := make(map[string][]string)
	instanceToBlock := make(map[string][]string)
	volumeSize := make(map[string]int64)

	addVolumeSize := func(id *string, size *int64) {
		if id == nil {
			return
		}
		if s := sanitizeVolumeSize(size); s != nil {
			volumeSize[*id] = *s
		}
	}

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
				instanceToBoot[*a.InstanceId] = append(instanceToBoot[*a.InstanceId], *a.BootVolumeId)
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
				addVolumeSize(v.Id, v.SizeInGBs)
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
				instanceToBlock[*instID] = append(instanceToBlock[*instID], *volID)
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
				addVolumeSize(v.Id, v.SizeInGBs)
			}
			if resp.OpcNextPage == nil {
				break
			}
			page = *resp.OpcNextPage
		}
	}

	bootTotals := sumInstanceVolumeSizes(instanceToBoot, volumeSize)
	blockTotals := sumInstanceVolumeSizes(instanceToBlock, volumeSize)

	out := make(map[string]instanceStorage, len(instanceToBoot)+len(instanceToBlock))
	for id, size := range bootTotals {
		size := size
		s := out[id]
		s.BootGB = &size
		out[id] = s
	}
	for id, size := range blockTotals {
		size := size
		s := out[id]
		s.BlockGB = &size
		out[id] = s
	}
	return out
}

// sumInstanceVolumeSizes totals, per instance, the sizes of every volume in
// instanceToVolumes (boot or block, whichever the caller passed) — given
// instanceID->volumeIDs and volumeID->sizeInGBs — split out from
// fetchInstanceStorage so this join/sum logic has a test that doesn't need
// a live SDK client. A volume whose size wasn't resolved (e.g. that AD's
// list call failed, or its size was excluded as implausible — see
// maxSaneVolumeGB) is skipped rather than counted as zero, so a partial
// failure undercounts instead of silently reporting a wrong total as if it
// were complete.
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
