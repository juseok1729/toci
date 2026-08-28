package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/oracle/oci-go-sdk/v65/core"
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
	shape string
	label string
}

// buildVcnDiagram assembles the currently VCN-filtered Instances, DB
// Systems, Autonomous DBs, Exadata VM Clusters, and any DRGs attached to
// the VCN into a Mermaid flowchart, grouped by the subnet each sits in. It's
// its own tea.Cmd (not called inline from the key handler) because it makes
// several fresh List calls independent of whatever's currently loaded in
// the table — Subnets, the VNIC-to-subnet join for Instances, the three
// database resources, and DRG attachments — so it belongs off the UI
// goroutine like any other API-driven action.
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

// attachedDrgIDs returns the OCIDs of every DRG attached to vcnID, via
// ListDrgAttachments(vcnId=...) — a DRG attachment is a compartment-level
// object, not a field on either the DRG or the VCN itself.
func attachedDrgIDs(ctx context.Context, vnClient core.VirtualNetworkClient, compartmentID, vcnID string) (map[string]bool, error) {
	out := make(map[string]bool)
	page := ""
	for {
		req := core.ListDrgAttachmentsRequest{CompartmentId: &compartmentID, VcnId: &vcnID}
		if page != "" {
			req.Page = &page
		}
		resp, err := vnClient.ListDrgAttachments(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, a := range resp.Items {
			if a.DrgId != nil {
				out[*a.DrgId] = true
			}
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return out, nil
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
	add := func(subnetID, shape, label string) {
		if _, ok := bySubnet[subnetID]; ok {
			bySubnet[subnetID] = append(bySubnet[subnetID], diagramNode{shape: shape, label: label})
		}
	}

	if computeClient, err := factory.Compute(scope.Region); err == nil {
		if instSubnets, err := registry.InstanceSubnetIDs(ctx, computeClient, scope.CompartmentID); err == nil {
			if instRows, err := fetchAll(ctx, registry.NewInstanceResource(factory), scope); err == nil {
				for _, row := range instRows {
					add(instSubnets[row.ID], "rect", row.Name)
				}
			}
		}
	}

	if dbRows, err := fetchAll(ctx, registry.NewDbSystemResource(factory), scope); err == nil {
		for _, row := range dbRows {
			if d, ok := row.Raw.(database.DbSystemSummary); ok {
				add(deref(d.SubnetId), "cylinder", row.Name)
			}
		}
	}

	if adbRows, err := fetchAll(ctx, registry.NewAutonomousDatabaseResource(factory), scope); err == nil {
		for _, row := range adbRows {
			if a, ok := row.Raw.(database.AutonomousDatabaseSummary); ok {
				add(deref(a.SubnetId), "cylinder", row.Name)
			}
		}
	}

	if exaRows, err := fetchAll(ctx, registry.NewCloudVmClusterResource(factory), scope); err == nil {
		for _, row := range exaRows {
			if c, ok := row.Raw.(database.CloudVmClusterSummary); ok {
				add(deref(c.SubnetId), "cylinder", row.Name)
			}
		}
	}

	var drgNames []string
	if vnClient, err := factory.VirtualNetwork(scope.Region); err == nil {
		if drgIDs, err := attachedDrgIDs(ctx, vnClient, scope.CompartmentID, scope.VcnID); err == nil && len(drgIDs) > 0 {
			if drgRows, err := fetchAll(ctx, registry.NewDrgResource(factory), registry.Scope{Region: scope.Region, CompartmentID: scope.CompartmentID}); err == nil {
				for _, row := range drgRows {
					if drgIDs[row.ID] {
						drgNames = append(drgNames, row.Name)
					}
				}
			}
		}
	}

	var b strings.Builder
	b.WriteString("graph TD\n")

	for i, name := range drgNames {
		fmt.Fprintf(&b, "  drg%d([%s])\n", i, mermaidLabel(name))
	}

	fmt.Fprintf(&b, "  subgraph vcn[%s]\n", mermaidLabel(vcnName))
	for i, subnetID := range subnetOrder {
		subNode := fmt.Sprintf("sub%d", i)
		fmt.Fprintf(&b, "    subgraph %s[%s]\n", subNode, mermaidLabel(subnetName[subnetID]))
		for j, n := range bySubnet[subnetID] {
			resNode := fmt.Sprintf("%s_%d", subNode, j)
			b.WriteString("      ")
			b.WriteString(shapedNode(resNode, n.shape, n.label))
			b.WriteString("\n")
		}
		b.WriteString("    end\n")
	}
	b.WriteString("  end\n")

	// Sibling subgraphs have no edges between them, so dagre treats each as
	// its own component and spreads them left-to-right — with several
	// subnets that runs the whole diagram off the page. Chaining them with
	// invisible links forces a top-to-bottom stack instead, bounding the
	// width to the widest single subnet rather than the sum of all of them.
	for i := 1; i < len(subnetOrder); i++ {
		fmt.Fprintf(&b, "  sub%d ~~~ sub%d\n", i-1, i)
	}

	for i := range drgNames {
		fmt.Fprintf(&b, "  drg%d --> vcn\n", i)
	}

	return b.String(), nil
}

func shapedNode(id, shape, label string) string {
	l := mermaidLabel(label)
	switch shape {
	case "cylinder":
		return fmt.Sprintf("%s[(%s)]", id, l)
	default:
		return fmt.Sprintf("%s[%s]", id, l)
	}
}

// mermaidLabel quotes a resource's display name and escapes embedded
// quotes — real OCI resource names are almost always hyphenated.
func mermaidLabel(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `#quot;`) + `"`
}
