package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/oracle/oci-go-sdk/v65/database"

	"toci/internal/clients"
	"toci/internal/registry"
)

type diagramMsg struct {
	path string
	err  error
}

// diagramNode is one leaf resource (Instance/DbSystem/ADB/Exadata) placed
// under a subnet in the generated diagram.
type diagramNode struct {
	icon  string
	label string
}

// buildVcnDiagram assembles the currently VCN-filtered Instances, DB
// Systems, Autonomous DBs, and Exadata VM Clusters into a Mermaid graph,
// grouped by the subnet each sits in. It's its own tea.Cmd (not called
// inline from the key handler) because it makes several fresh List calls
// independent of whatever's currently loaded in the table — Subnets, the
// VNIC-to-subnet join for Instances, and the three database resources —
// so it belongs off the UI goroutine like any other API-driven action.
func (m Model) buildVcnDiagram() tea.Cmd {
	factory := m.factory
	scope := m.scope // already carries VcnID from the active VCN filter
	vcnName := m.vcnFilterName

	return func() tea.Msg {
		ctx := context.Background()
		content, err := renderVcnMermaid(ctx, factory, scope, vcnName)
		if err != nil {
			return diagramMsg{err: err}
		}
		safeName := filenameUnsafe.ReplaceAllString(vcnName, "-")
		path := fmt.Sprintf("toci-diagram-%s-%s.mmd", safeName, time.Now().Format("20060102-150405"))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return diagramMsg{err: err}
		}
		return diagramMsg{path: path}
	}
}

func renderVcnMermaid(ctx context.Context, factory *clients.Factory, scope registry.Scope, vcnName string) (string, error) {
	subnetRows, err := fetchAll(ctx, registry.NewSubnetResource(factory), scope)
	if err != nil {
		return "", fmt.Errorf("list subnets: %w", err)
	}

	bySubnet := make(map[string][]diagramNode, len(subnetRows))
	subnetOrder := make([]string, 0, len(subnetRows))
	subnetName := make(map[string]string, len(subnetRows))
	for _, row := range subnetRows {
		subnetOrder = append(subnetOrder, row.ID)
		subnetName[row.ID] = row.Name
		bySubnet[row.ID] = nil
	}
	add := func(subnetID, icon, label string) {
		if _, ok := bySubnet[subnetID]; ok {
			bySubnet[subnetID] = append(bySubnet[subnetID], diagramNode{icon: icon, label: label})
		}
	}

	if computeClient, err := factory.Compute(scope.Region); err == nil {
		if instSubnets, err := registry.InstanceSubnetIDs(ctx, computeClient, scope.CompartmentID); err == nil {
			if instRows, err := fetchAll(ctx, registry.NewInstanceResource(factory), scope); err == nil {
				for _, row := range instRows {
					add(instSubnets[row.ID], "🖥", row.Name)
				}
			}
		}
	}

	if dbRows, err := fetchAll(ctx, registry.NewDbSystemResource(factory), scope); err == nil {
		for _, row := range dbRows {
			if d, ok := row.Raw.(database.DbSystemSummary); ok {
				add(deref(d.SubnetId), "🗄", row.Name)
			}
		}
	}

	if adbRows, err := fetchAll(ctx, registry.NewAutonomousDatabaseResource(factory), scope); err == nil {
		for _, row := range adbRows {
			if a, ok := row.Raw.(database.AutonomousDatabaseSummary); ok {
				add(deref(a.SubnetId), "☁", row.Name)
			}
		}
	}

	if exaRows, err := fetchAll(ctx, registry.NewCloudVmClusterResource(factory), scope); err == nil {
		for _, row := range exaRows {
			if c, ok := row.Raw.(database.CloudVmClusterSummary); ok {
				add(deref(c.SubnetId), "💾", row.Name)
			}
		}
	}

	var b strings.Builder
	b.WriteString("graph TD\n")
	fmt.Fprintf(&b, "  VCN[\"🌐 %s\"]\n", mermaidEscape(vcnName))
	for i, subnetID := range subnetOrder {
		subNode := fmt.Sprintf("SUB%d", i)
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", subNode, mermaidEscape(subnetName[subnetID]))
		fmt.Fprintf(&b, "  VCN --> %s\n", subNode)
		for j, n := range bySubnet[subnetID] {
			resNode := fmt.Sprintf("%s_%d", subNode, j)
			fmt.Fprintf(&b, "  %s[\"%s %s\"]\n", resNode, n.icon, mermaidEscape(n.label))
			fmt.Fprintf(&b, "  %s --> %s\n", subNode, resNode)
		}
	}
	return b.String(), nil
}

// mermaidEscape keeps a resource's display name from breaking out of its
// quoted node label.
func mermaidEscape(s string) string {
	return strings.ReplaceAll(s, `"`, `#quot;`)
}
