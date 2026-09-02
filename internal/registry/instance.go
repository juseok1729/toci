package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

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
	Metrics instanceMetrics
	IPs     instanceIPs
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

func ipString(ip string) string {
	if ip == "" {
		return "-"
	}
	return ip
}

// shortAD trims the tenancy-prefixed availability domain (e.g.
// "kIQq:AP-SEOUL-1-AD-1") down to just "AD-1" — the region qualifier is
// redundant since everything shown is already scoped to one region.
func shortAD(ad string) string {
	if i := strings.LastIndex(ad, "AD-"); i >= 0 {
		return ad[i:]
	}
	return ad
}

// shortFD trims "FAULT-DOMAIN-2" down to "FD-2".
func shortFD(fd string) string {
	return strings.Replace(fd, "FAULT-DOMAIN-", "FD-", 1)
}

func (r *InstanceResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(instanceRow).DisplayName)
		}},
		{Header: "STATE", Width: 10, Get: func(row Row) string {
			return stateLabel(row.Raw.(instanceRow).LifecycleState)
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
		{Header: "USAGE(CPU/MEM %)", Width: 18, Get: func(row Row) string {
			m := row.Raw.(instanceRow).Metrics
			return pctString(m.CPUPercent) + "/" + pctString(m.MemPercent)
		}},
		{Header: "DOMAIN(AD/FD)", Width: 14, Get: func(row Row) string {
			inst := row.Raw.(instanceRow).Instance
			return shortAD(deref(inst.AvailabilityDomain)) + "/" + shortFD(deref(inst.FaultDomain))
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

	var metrics map[string]instanceMetrics
	if monClient, err := r.factory.Monitoring(s.Region); err == nil {
		metrics = fetchInstanceMetrics(ctx, monClient, s.CompartmentID)
	}

	vnClient, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	ips := fetchInstanceIPs(ctx, client, vnClient, s.CompartmentID)

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
			Instance: i,
			Metrics:  metrics[id],
			IPs:      ips[id],
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
	}
}

func (r *InstanceResource) RunAction(ctx context.Context, s Scope, key, id string) error {
	client, err := r.factory.Compute(s.Region)
	if err != nil {
		return err
	}

	var action core.InstanceActionActionEnum
	switch key {
	case "start":
		action = core.InstanceActionActionStart
	case "stop":
		action = core.InstanceActionActionSoftstop
	default:
		return fmt.Errorf("unknown instance action %q", key)
	}

	_, err = client.InstanceAction(ctx, core.InstanceActionRequest{
		InstanceId: &id,
		Action:     action,
	})
	return err
}
