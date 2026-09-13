package app

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// compartmentTreeMsg carries the once-at-startup full compartment tree
// fetch (see fetchCompartmentTree) back to Update.
type compartmentTreeMsg struct {
	tree *compartmentTree
	err  error
}

func (m Model) fetchCompartmentTreeCmd() tea.Cmd {
	factory := m.factory
	region := m.scope.Region
	tenancyID := m.compPath[0].ID
	rootName := m.compPath[0].Name
	return func() tea.Msg {
		tree, err := fetchCompartmentTree(context.Background(), factory, region, tenancyID, rootName)
		return compartmentTreeMsg{tree: tree, err: err}
	}
}

// handleCompartmentTreeMsg is Update's case for compartmentTreeMsg.
func (m Model) handleCompartmentTreeMsg(msg compartmentTreeMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.statusMsg = "compartment tree load failed: " + msg.err.Error()
		return m, nil
	}
	m.compTree = msg.tree
	if m.tenancyName != "" {
		if n := m.compTree.node(m.compTree.root.ID); n != nil {
			n.Name = m.tenancyName
		}
	}
	if path := m.compTree.pathTo(m.scope.CompartmentID); len(path) > 0 {
		m.compPath = path
		m.relayout()
	}
	return m, nil
}

// openCompartmentPicker opens the F5 tree picker (the "c" key, from any
// resource view). On the Compartments resource view itself, the row under
// the cursor is preselected (F6's "c pre-selects the current row").
func (m *Model) openCompartmentPicker() {
	if m.compTree == nil {
		m.statusMsg = "compartment tree still loading — try again shortly"
		return
	}
	preselect := m.scope.CompartmentID
	if m.current().Key() == "compartment" {
		if row, ok := m.selected(); ok {
			preselect = row.ID
		}
	}
	m.picker = m.newCompartmentPicker(m.scope.CompartmentID, preselect)
	m.mode = modePicker
}

// boolPtr is switchCompartment's way of distinguishing "leave subtree mode
// as it is" (nil) from "force it to this state" (non-nil).
func boolPtr(b bool) *bool { return &b }

// switchCompartment is F1's core: swap the active compartment without
// touching the current resource type, resetting any VCN/DRG sub-scope
// (doc: "컴파트먼트 전환 시 하위 스코프는 초기화한다"), and reload. subtree, when
// non-nil, forces subtree mode to that state (Tab in the picker forces it
// on; a Recent hotkey restores whatever state it was saved in); nil keeps
// whatever subtree mode was already active.
func (m *Model) switchCompartment(id, name string, subtree *bool) tea.Cmd {
	if subtree != nil {
		m.subtreeOn = *subtree
	}
	if m.subtreeOn && m.compTree == nil {
		m.subtreeOn = false
		m.statusMsg = "subtree mode needs the compartment tree, which is still loading — try again shortly"
	}
	if !m.subtreeOn {
		m.stopSubtreeFanout()
	}

	// Doc F1: "커서 위치는 가능하면 유지(동일 리소스 ID가 있으면 커서 복원)" — recorded
	// here and applied once the reload's rowsMsg lands (see its handler).
	m.restoreCursorID = ""
	if row, ok := m.selected(); ok {
		m.restoreCursorID = row.ID
	}

	m.scope.CompartmentID = id
	m.scope.VcnID = ""
	m.vcnFilterName = ""
	m.scope.DrgID = ""
	m.drgFilterName = ""
	m.vcnNames = nil
	m.err = nil

	// relayout() below re-renders the table against m.displayRows using
	// m.displayColumns() for the (possibly just-changed) subtreeOn state —
	// if subtree mode just turned off while a placeholder row was still
	// pending, the *old* displayRows would otherwise get run through the
	// *new*, unwrapped columns (whose Get type-asserts a real Row.Raw and
	// panics on a placeholder's nil one). Resync first so relayout always
	// sees a set of rows that actually matches the current mode.
	m.rows = nil
	m.setDisplayRows()

	if m.compTree != nil {
		if path := m.compTree.pathTo(id); len(path) > 0 {
			m.compPath = path
		} else {
			m.compPath = []crumb{{ID: id, Name: name}}
		}
	} else {
		m.compPath = []crumb{{ID: id, Name: name}}
	}
	m.relayout()

	m.recentList = pushRecent(m.recentList, id, name, m.subtreeOn)
	saveRecent(m.profile, m.recentList)

	if m.subtreeOn {
		return m.startSubtreeFanout()
	}
	m.loading = true
	return m.load()
}
