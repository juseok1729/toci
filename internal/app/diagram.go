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
// under a subnet in the generated diagram. icon must be one of
// architecture-beta's built-in icons (cloud/database/disk/internet/server)
// — anything else needs an external icon pack registered in whatever
// renders the .mmd, which we can't assume.
type diagramNode struct {
	icon  string
	label string
}

// buildVcnDiagram assembles the currently VCN-filtered Instances, DB
// Systems, Autonomous DBs, Exadata VM Clusters, and any DRGs attached to
// the VCN into a Mermaid architecture-beta diagram, grouped by the subnet
// each sits in (DRGs sit outside the VCN group, connected to it — they
// attach to the VCN as a whole, not to any one subnet). It's its own
// tea.Cmd (not called inline from the key handler) because it makes
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
	add := func(subnetID, icon, label string) {
		if _, ok := bySubnet[subnetID]; ok {
			bySubnet[subnetID] = append(bySubnet[subnetID], diagramNode{icon: icon, label: label})
		}
	}

	if computeClient, err := factory.Compute(scope.Region); err == nil {
		if instSubnets, err := registry.InstanceSubnetIDs(ctx, computeClient, scope.CompartmentID); err == nil {
			if instRows, err := fetchAll(ctx, registry.NewInstanceResource(factory), scope); err == nil {
				for _, row := range instRows {
					add(instSubnets[row.ID], "server", row.Name)
				}
			}
		}
	}

	if dbRows, err := fetchAll(ctx, registry.NewDbSystemResource(factory), scope); err == nil {
		for _, row := range dbRows {
			if d, ok := row.Raw.(database.DbSystemSummary); ok {
				add(deref(d.SubnetId), "database", row.Name)
			}
		}
	}

	if adbRows, err := fetchAll(ctx, registry.NewAutonomousDatabaseResource(factory), scope); err == nil {
		for _, row := range adbRows {
			if a, ok := row.Raw.(database.AutonomousDatabaseSummary); ok {
				add(deref(a.SubnetId), "database", row.Name)
			}
		}
	}

	if exaRows, err := fetchAll(ctx, registry.NewCloudVmClusterResource(factory), scope); err == nil {
		for _, row := range exaRows {
			if c, ok := row.Raw.(database.CloudVmClusterSummary); ok {
				add(deref(c.SubnetId), "database", row.Name)
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
	b.WriteString("architecture-beta\n")

	for i, name := range drgNames {
		fmt.Fprintf(&b, "    service drg%d(internet)%s\n", i, mermaidLabel(name))
	}

	fmt.Fprintf(&b, "    group vcn(cloud)%s\n", mermaidLabel(vcnName))
	// vcnhub anchors a layout chain through every DRG and subnet below —
	// without *some* edges, architecture-beta's grid layout has nothing to
	// position sibling subnet groups from and stacks them on top of each
	// other (confirmed live: 5 subnets with no edges rendered fully
	// overlapping in Notion). A star (everything connected straight to the
	// hub) was the first fix, but with only 4 sides (R/L/T/B) a 5th+ subnet
	// has to reuse a side already claimed by another — confirmed live too:
	// subnet 0 and subnet 4 landed on the same "R" port and collided the
	// same way. A chain — vcnhub → drg0 → drg1 → sub0 → sub1 → ... — never
	// reuses a port on the same node no matter how many there are, since
	// each hop is a fresh pair of nodes.
	b.WriteString("    junction vcnhub in vcn\n")

	// anchor per subnet: its first service if it has one, otherwise its own
	// junction (an empty subnet has nothing else to hang an edge off of).
	subnetAnchor := make([]string, len(subnetOrder))
	for i, subnetID := range subnetOrder {
		subNode := fmt.Sprintf("sub%d", i)
		fmt.Fprintf(&b, "    group %s(cloud)%s in vcn\n", subNode, mermaidLabel(subnetName[subnetID]))
		leaves := bySubnet[subnetID]
		for j, n := range leaves {
			resNode := fmt.Sprintf("%s_%d", subNode, j)
			fmt.Fprintf(&b, "    service %s(%s)%s in %s\n", resNode, n.icon, mermaidLabel(n.label), subNode)
		}
		if len(leaves) > 0 {
			subnetAnchor[i] = subNode + "_0"
		} else {
			hub := subNode + "hub"
			fmt.Fprintf(&b, "    junction %s in %s\n", hub, subNode)
			subnetAnchor[i] = hub
		}
	}

	sides := [4][2]string{{"R", "L"}, {"L", "R"}, {"T", "B"}, {"B", "T"}}
	prev, step := "vcnhub", 0
	chain := func(to string) {
		side := sides[step%len(sides)]
		fmt.Fprintf(&b, "    %s:%s --> %s:%s\n", prev, side[0], side[1], to)
		prev, step = to, step+1
	}
	for i := range drgNames {
		chain(fmt.Sprintf("drg%d", i))
	}
	for _, anchor := range subnetAnchor {
		chain(anchor)
	}

	return b.String(), nil
}

// mermaidLabel quotes a resource's display name (as architecture-beta's
// own [label] syntax expects unquoted text and chokes on hyphens — real
// OCI resource names are almost always hyphenated, e.g.
// "wyd-logistics-drg") and escapes embedded quotes. Returns the bracketed
// [label] form ready to append directly after a service/group's (icon).
func mermaidLabel(s string) string {
	return `["` + strings.ReplaceAll(s, `"`, `#quot;`) + `"]`
}
