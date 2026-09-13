package registry

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/computeinstanceagent"
	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

// computeInstanceMonitoringPlugin is the exact Oracle Cloud Agent plugin
// name for the "Compute Instance Monitoring" plugin — the one that
// publishes CpuUtilization/MemoryUtilization/etc. to the oci_computeagent
// namespace (see instance_metrics.go). Must match the plugin name OCI's
// own API expects verbatim; there's no enum for it, just this string.
const computeInstanceMonitoringPlugin = "Compute Instance Monitoring"

// bastionAgentPlugin is the exact Oracle Cloud Agent plugin name for the
// "Bastion" plugin — required on the target instance before an OCI Bastion
// managed-SSH session can be created against it.
const bastionAgentPlugin = "Bastion"

type InstanceResource struct {
	factory *clients.Factory
}

func NewInstanceResource(f *clients.Factory) *InstanceResource {
	return &InstanceResource{factory: f}
}

func (r *InstanceResource) Key() string   { return "instance" }
func (r *InstanceResource) Label() string { return "Instances" }

// instanceRow is what List stores in Row.Raw: the SDK instance plus the
// metrics/IP samples fetched alongside it, so Columns and the detail view
// can both read from a single value.
type instanceRow struct {
	core.Instance
	Metrics    instanceMetrics
	IPs        instanceIPs
	Storage    instanceStorage
	SubnetName string
	OSVersion  string
}

func pctString(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", *v)
}

func floatString(v *float32) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.1f", *v)
}

func int64String(v *int64) string {
	if v == nil {
		return "-"
	}
	return itoa(int(*v))
}

func ipString(ip string) string {
	if ip == "" {
		return "-"
	}
	return ip
}

func (r *InstanceResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(instanceRow).DisplayName)
		}},
		{Header: "STATE", Width: 10, Get: func(row Row) string {
			return stateLabel(row.Raw.(instanceRow).LifecycleState)
		}},
		{Header: "OS", Width: 14, Get: func(row Row) string {
			if v := row.Raw.(instanceRow).OSVersion; v != "" {
				return v
			}
			return "-"
		}},
		{Header: "SUBNET", Width: 24, Get: func(row Row) string {
			if v := row.Raw.(instanceRow).SubnetName; v != "" {
				return v
			}
			return "-"
		}},
		{Header: "PUBLIC IP", Width: 15, Get: func(row Row) string {
			return ipString(row.Raw.(instanceRow).IPs.Public)
		}},
		{Header: "PRIVATE IP", Width: 15, Get: func(row Row) string {
			return ipString(row.Raw.(instanceRow).IPs.Private)
		}},
		{Header: "SHAPE", Width: 20, Get: func(row Row) string {
			return deref(row.Raw.(instanceRow).Shape)
		}},
		{Header: "OCPU", Width: 6, Get: func(row Row) string {
			cfg := row.Raw.(instanceRow).ShapeConfig
			if cfg == nil {
				return "-"
			}
			return floatString(cfg.Ocpus)
		}},
		{Header: "MEM(GB)", Width: 8, Get: func(row Row) string {
			cfg := row.Raw.(instanceRow).ShapeConfig
			if cfg == nil {
				return "-"
			}
			return floatString(cfg.MemoryInGBs)
		}},
		{Header: "BOOT/BLK(GB)", Width: 12, Get: func(row Row) string {
			s := row.Raw.(instanceRow).Storage
			return int64String(s.BootGB) + "/" + int64String(s.BlockGB)
		}},
		{Header: "CPU/MEM%", Width: 10, Get: func(row Row) string {
			m := row.Raw.(instanceRow).Metrics
			return pctString(m.CPUPercent) + "/" + pctString(m.MemPercent)
		}},
	}
}

func (r *InstanceResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.Compute(s.Region)
	if err != nil {
		return nil, "", err
	}

	req := core.ListInstancesRequest{CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListInstances(ctx, req)
	if err != nil {
		return nil, "", err
	}

	vnClient, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	// Metrics, IPs, storage, subnet membership/names, and image labels are
	// independent lookups over the same compartment/instance list (each
	// already best-effort — a failure just leaves that data blank rather
	// than failing the listing) — fetched concurrently instead of one
	// after another.
	var (
		metrics       map[string]instanceMetrics
		ips           map[string]instanceIPs
		storage       map[string]instanceStorage
		instSubnetIDs map[string]string
		subnetLabels  map[string]string
		imageLabels   map[string]string
		wg            sync.WaitGroup
	)
	wg.Add(5)
	go func() {
		defer wg.Done()
		if monClient, err := r.factory.Monitoring(s.Region); err == nil {
			metrics = fetchInstanceMetrics(ctx, monClient, s.CompartmentID)
		}
	}()
	go func() {
		defer wg.Done()
		ips = fetchInstanceIPs(ctx, client, vnClient, s.CompartmentID)
	}()
	go func() {
		defer wg.Done()
		if bsClient, err := r.factory.Blockstorage(s.Region); err == nil {
			ads := make([]string, 0, len(resp.Items))
			for _, i := range resp.Items {
				ads = append(ads, deref(i.AvailabilityDomain))
			}
			storage = fetchInstanceStorage(ctx, client, bsClient, s.CompartmentID, ads)
		}
	}()
	go func() {
		defer wg.Done()
		instSubnetIDs, _ = InstanceSubnetIDs(ctx, client, s.CompartmentID)
		subnetLabels, _ = subnetNames(ctx, vnClient, s.CompartmentID)
	}()
	go func() {
		defer wg.Done()
		imageLabels = fetchImageLabels(ctx, client, resp.Items)
	}()
	wg.Wait()

	var allow map[string]bool
	if s.VcnID != "" {
		allow, err = instanceIDsInVcn(ctx, vnClient, client, s.CompartmentID, s.VcnID)
		if err != nil {
			return nil, "", err
		}
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, i := range resp.Items {
		id := deref(i.Id)
		if allow != nil && !allow[id] {
			continue
		}
		rows = append(rows, Row{ID: id, Name: deref(i.DisplayName), TimeCreated: timeOf(i.TimeCreated), Raw: instanceRow{
			Instance:   i,
			Metrics:    metrics[id],
			IPs:        ips[id],
			Storage:    storage[id],
			SubnetName: subnetLabels[instSubnetIDs[id]],
			OSVersion:  imageLabels[deref(i.ImageId)],
		}})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}

func (r *InstanceResource) Actions() []ActionSpec {
	return []ActionSpec{
		{Key: "start", Label: "Start"},
		{Key: "stop", Label: "Stop (graceful)"},
		{Key: "enable-monitoring", Label: "Enable Monitoring"},
		{Key: "enable-bastion-plugin", Label: "Enable Bastion Plugin"},
		{Key: "plugin-status", Label: "Check Plugin Status"},
	}
}

func (r *InstanceResource) RunAction(ctx context.Context, s Scope, key, id string) (string, error) {
	if key == "plugin-status" {
		client, err := r.factory.InstanceAgentPlugin(s.Region)
		if err != nil {
			return "", err
		}
		resp, err := client.ListInstanceAgentPlugins(ctx, computeinstanceagent.ListInstanceAgentPluginsRequest{
			CompartmentId:   &s.CompartmentID,
			InstanceagentId: &id,
		})
		if err != nil {
			return "", err
		}
		if len(resp.Items) == 0 {
			return "no plugins reported (Oracle Cloud Agent may not be running)", nil
		}
		parts := make([]string, len(resp.Items))
		for i, p := range resp.Items {
			parts[i] = deref(p.Name) + ": " + string(p.Status)
		}
		return strings.Join(parts, "\n"), nil
	}

	client, err := r.factory.Compute(s.Region)
	if err != nil {
		return "", err
	}

	if key == "enable-monitoring" {
		// isMonitoringDisabled=false unblocks the whole monitoring
		// category (OCI docs: "if isMonitoringDisabled is true, all
		// monitoring plugins are disabled regardless of per-plugin
		// config") and the pluginsConfig entry enables this specific
		// plugin — needed both together to guarantee it turns on
		// regardless of the instance's current state. Omitting other
		// plugins from pluginsConfig leaves their own state untouched
		// (OCI updates plugins individually, not as a full-list replace).
		_, err = client.UpdateInstance(ctx, core.UpdateInstanceRequest{
			InstanceId: &id,
			UpdateInstanceDetails: core.UpdateInstanceDetails{
				AgentConfig: &core.UpdateInstanceAgentConfigDetails{
					IsMonitoringDisabled: common.Bool(false),
					PluginsConfig: []core.InstanceAgentPluginConfigDetails{
						{
							Name:         common.String(computeInstanceMonitoringPlugin),
							DesiredState: core.InstanceAgentPluginConfigDetailsDesiredStateEnabled,
						},
					},
				},
			},
		})
		return "", err
	}

	if key == "enable-bastion-plugin" {
		// Bastion isn't a monitoring or management plugin, so it's gated
		// only by the master AreAllPluginsDisabled switch, not
		// IsMonitoringDisabled/IsManagementDisabled.
		_, err = client.UpdateInstance(ctx, core.UpdateInstanceRequest{
			InstanceId: &id,
			UpdateInstanceDetails: core.UpdateInstanceDetails{
				AgentConfig: &core.UpdateInstanceAgentConfigDetails{
					AreAllPluginsDisabled: common.Bool(false),
					PluginsConfig: []core.InstanceAgentPluginConfigDetails{
						{
							Name:         common.String(bastionAgentPlugin),
							DesiredState: core.InstanceAgentPluginConfigDetailsDesiredStateEnabled,
						},
					},
				},
			},
		})
		return "", err
	}

	var action core.InstanceActionActionEnum
	switch key {
	case "start":
		action = core.InstanceActionActionStart
	case "stop":
		action = core.InstanceActionActionSoftstop
	default:
		return "", fmt.Errorf("unknown instance action %q", key)
	}

	_, err = client.InstanceAction(ctx, core.InstanceActionRequest{
		InstanceId: &id,
		Action:     action,
	})
	return "", err
}
