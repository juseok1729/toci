package registry

import (
	"context"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

// instanceMetrics is the latest CPU/memory utilization sample for one
// instance, as reported by the OCI Compute Agent monitoring plugin. Either
// field is nil if the agent plugin isn't installed/enabled on that instance.
type instanceMetrics struct {
	CPUPercent *float64
	MemPercent *float64
}

// fetchInstanceMetrics queries the last 10 minutes of CpuUtilization and
// MemoryUtilization for every instance in the compartment with two
// SummarizeMetricsData calls (not one per instance), keyed by instance OCID.
// A query failure (e.g. no monitoring permission) just yields empty metrics
// rather than failing the instance listing.
func fetchInstanceMetrics(ctx context.Context, client monitoring.MonitoringClient, compartmentID string) map[string]instanceMetrics {
	out := make(map[string]instanceMetrics)
	end := common.SDKTime{Time: time.Now()}
	start := common.SDKTime{Time: end.Time.Add(-10 * time.Minute)}

	query := func(mql string, assign func(*instanceMetrics, float64)) {
		resp, err := client.SummarizeMetricsData(ctx, monitoring.SummarizeMetricsDataRequest{
			CompartmentId: &compartmentID,
			SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
				Namespace: common.String("oci_computeagent"),
				Query:     common.String(mql),
				StartTime: &start,
				EndTime:   &end,
			},
		})
		if err != nil {
			return
		}
		for _, md := range resp.Items {
			id, ok := md.Dimensions["resourceId"]
			if !ok || len(md.AggregatedDatapoints) == 0 {
				continue
			}
			last := md.AggregatedDatapoints[len(md.AggregatedDatapoints)-1]
			if last.Value == nil {
				continue
			}
			m := out[id]
			assign(&m, *last.Value)
			out[id] = m
		}
	}

	query("CpuUtilization[1m].mean()", func(m *instanceMetrics, v float64) { m.CPUPercent = &v })
	query("MemoryUtilization[1m].mean()", func(m *instanceMetrics, v float64) { m.MemPercent = &v })

	return out
}
