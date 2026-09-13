package app

import (
	"context"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"

	"toci/internal/clients"
)

// compartmentNode is one entry in the cached compartment tree — see
// compartmentTree's doc for why this is fetched once and cached rather than
// paged live from each Compartments view.
type compartmentNode struct {
	ID          string
	Name        string
	Description string
	ParentID    string
	State       identity.CompartmentLifecycleStateEnum
	Children    []*compartmentNode
}

// compartmentTree is toci's in-memory model of the tenancy's compartment
// hierarchy, built once at startup (see fetchCompartmentTree) from a single
// ListCompartments(compartmentIdInSubtree=true, accessLevel=ACCESSIBLE)
// call — every compartment context switch (F1), the tree picker (F5), and
// subtree fan-out (F3) read from this cache instead of re-listing.
type compartmentTree struct {
	root *compartmentNode
	byID map[string]*compartmentNode
}

// fetchCompartmentTree pages through every compartment the caller can see
// (ACCESSIBLE — including ones reachable only via a resource in a
// subcompartment, not just direct INSPECT grants) and links them into a
// tree rooted at tenancyID.
func fetchCompartmentTree(ctx context.Context, factory *clients.Factory, region, tenancyID, rootName string) (*compartmentTree, error) {
	client, err := factory.Identity(region)
	if err != nil {
		return nil, err
	}

	byID := map[string]*compartmentNode{
		tenancyID: {ID: tenancyID, Name: rootName, State: identity.CompartmentLifecycleStateActive},
	}

	page := ""
	for {
		req := identity.ListCompartmentsRequest{
			CompartmentId:          &tenancyID,
			CompartmentIdInSubtree: common.Bool(true),
			AccessLevel:            identity.ListCompartmentsAccessLevelAccessible,
			LifecycleState:         identity.CompartmentLifecycleStateActive,
		}
		if page != "" {
			req.Page = &page
		}
		resp, err := client.ListCompartments(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, c := range resp.Items {
			byID[deref(c.Id)] = &compartmentNode{
				ID:          deref(c.Id),
				Name:        deref(c.Name),
				Description: deref(c.Description),
				ParentID:    deref(c.CompartmentId),
				State:       c.LifecycleState,
			}
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}

	root := byID[tenancyID]
	for id, n := range byID {
		if id == tenancyID {
			continue
		}
		parent, ok := byID[n.ParentID]
		if !ok {
			parent = root // orphaned (parent outside our access) — hang it off the root
		}
		parent.Children = append(parent.Children, n)
	}
	return &compartmentTree{root: root, byID: byID}, nil
}

func (t *compartmentTree) node(id string) *compartmentNode {
	if t == nil {
		return nil
	}
	return t.byID[id]
}

// pathTo returns the ancestor chain from the tree's root down to id
// (inclusive), for rendering the "Compartment:" breadcrumb.
func (t *compartmentTree) pathTo(id string) []crumb {
	n := t.node(id)
	if n == nil {
		return nil
	}
	var chain []*compartmentNode
	for cur := n; cur != nil; cur = t.node(cur.ParentID) {
		chain = append(chain, cur)
		if cur == t.root {
			break
		}
	}
	path := make([]crumb, len(chain))
	for i, n := range chain {
		path[len(chain)-1-i] = crumb{ID: n.ID, Name: n.Name}
	}
	return path
}

// descendants flattens every compartment under id (id itself excluded), in
// no particular order.
func (n *compartmentNode) descendants() []*compartmentNode {
	var out []*compartmentNode
	var walk func(*compartmentNode)
	walk = func(cur *compartmentNode) {
		for _, c := range cur.Children {
			out = append(out, c)
			walk(c)
		}
	}
	walk(n)
	return out
}

// subtreeCount is len(descendants()), for the picker's "⊕ n" badge.
func (n *compartmentNode) subtreeCount() int {
	return len(n.descendants())
}

// relativePath renders id's path relative to baseID (baseID's own
// descendants only — a joined "/" of names from baseID's child down to id),
// e.g. baseID="hub-and-spoke", id="hub-and-spoke/db/backup" -> "db/backup".
// Returns "" if id isn't under baseID (including id == baseID).
func (t *compartmentTree) relativePath(baseID, id string) string {
	n := t.node(id)
	if n == nil || id == baseID {
		return ""
	}
	var names []string
	for cur := n; cur != nil && cur.ID != baseID; cur = t.node(cur.ParentID) {
		names = append([]string{cur.Name}, names...)
		if cur.ParentID == "" {
			return "" // walked off the top without finding baseID
		}
	}
	return strings.Join(names, "/")
}
