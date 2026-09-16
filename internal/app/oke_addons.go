package app

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/oracle/oci-go-sdk/v65/containerengine"

	"toci/internal/registry"
)

// fetchOkeAddons lists an OKE cluster's installed add-ons and their
// lifecycle state — the "Add-ons" item on the picker Enter on an "oke" row
// opens (see openOkeResourceSearch). Reuses actionResultMsg (same shape
// Instance's "Check Plugin Status" action produces) so the existing
// actionResultMsg handler's "show it in modeDetail" path needs no change,
// just its own colorizer (colorizeAddonStatusText, via the colorize field)
// for OKE's own Active/Failed/Needs Attention states.
func (m Model) fetchOkeAddons(row registry.Row) tea.Cmd {
	factory := m.factory
	region := m.scope.Region
	label := "Add-ons: " + row.Name
	clusterID := row.ID
	return func() tea.Msg {
		client, err := factory.ContainerEngine(region)
		if err != nil {
			return actionResultMsg{label: label, err: err}
		}
		resp, err := client.ListAddons(context.Background(), containerengine.ListAddonsRequest{ClusterId: &clusterID})
		if err != nil {
			return actionResultMsg{label: label, err: err}
		}
		if len(resp.Items) == 0 {
			// actionResultMsg's handler only opens the detail view for a
			// non-empty msg — an empty string here would instead fall
			// through to its "fire and forget" branch (which reloads the
			// current table, the wrong thing to do for a status view).
			return actionResultMsg{label: label, msg: "(no add-ons installed)"}
		}
		lines := make([]string, len(resp.Items))
		for i, a := range resp.Items {
			lines[i] = fmt.Sprintf("%s: %s", deref(a.Name), registry.StateLabel(a.LifecycleState))
		}
		return actionResultMsg{label: label, msg: strings.Join(lines, "\n"), colorize: colorizeAddonStatusText}
	}
}
