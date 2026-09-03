package registry

import (
	"context"
	"sync"

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
//
// Each instance's GetVnic call(s) run in their own goroutine — a
// compartment with many instances used to pay for this one round trip at a
// time. Results funnel through a channel into a single map-writing
// goroutine rather than writing the shared map from every goroutine
// directly, since a plain Go map isn't safe for concurrent writes even to
// different keys (unlike db_system.go's List, which writes to a
// pre-sized slice by index instead).
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

	type result struct {
		instanceID string
		ips        instanceIPs
	}
	results := make(chan result, len(byInstance))
	var wg sync.WaitGroup
	for instanceID, vnicIDs := range byInstance {
		wg.Add(1)
		go func(instanceID string, vnicIDs []string) {
			defer wg.Done()
			var fallback *instanceIPs
			for _, vnicID := range vnicIDs {
				vnicID := vnicID
				resp, err := vnClient.GetVnic(ctx, core.GetVnicRequest{VnicId: &vnicID})
				if err != nil {
					continue
				}
				ips := instanceIPs{Public: deref(resp.PublicIp), Private: deref(resp.PrivateIp)}
				if resp.IsPrimary != nil && *resp.IsPrimary {
					results <- result{instanceID, ips}
					return
				}
				if fallback == nil {
					fallback = &ips
				}
			}
			if fallback != nil {
				results <- result{instanceID, *fallback}
			}
		}(instanceID, vnicIDs)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	out := make(map[string]instanceIPs, len(byInstance))
	for r := range results {
		out[r.instanceID] = r.ips
	}
	return out
}
