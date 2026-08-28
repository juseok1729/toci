// Package app holds the bubbletea model: state machine and view for toci.
package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"gopkg.in/yaml.v3"

	"toci/internal/clients"
	"toci/internal/registry"
)

var (
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	titleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	pathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(0, 1)
	selStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("39"))
)

type mode int

const (
	modeTable mode = iota
	modeDetail
	modePicker
	modeFilter
	modeConfirm
	modePrompt
	modeSidebar
)

type rowsMsg struct {
	rows []registry.Row
	err  error
}

type rootNameMsg struct{ name string }

type regionsMsg struct {
	items []pickerItem
	err   error
}

type actionResultMsg struct {
	label string
	err   error
}

type bastionsMsg struct {
	items []pickerItem
	err   error
}

type sessionReadyMsg struct {
	sshCmd string
	err    error
}

type sshDoneMsg struct{ err error }

type Model struct {
	factory   *clients.Factory
	profile   string
	resources []registry.Resource
	resIdx    int
	scope     registry.Scope
	compPath  []crumb

	rows        []registry.Row
	displayRows []registry.Row
	filterQuery string
	filterBak   string

	table       table.Model
	detail      viewport.Model
	picker      picker
	filterInput textinput.Model

	regionItems []pickerItem

	writeEnabled  bool
	pendingRow    registry.Row
	pendingAction registry.ActionSpec
	confirmInput  textinput.Model

	sshBastionID string
	promptInput  textinput.Model

	mode          mode
	sidebarCursor int
	loading       bool
	err           error
	statusMsg     string

	// autoRedirect is set right before a load that's allowed to jump away
	// to VCNs if it comes back empty — descending into a compartment,
	// basically. A manual switch back to Compartments (sidebar/Tab) must
	// NOT set this, or an already-empty compartment bounces straight back
	// to VCNs and the user can never re-pick a sibling compartment.
	autoRedirect bool

	// vcnFilterName is non-empty while the Instance table is scoped to one
	// VCN (scope.VcnID set) via the "i" key on a VCN row — shown in the
	// header and used to know what Esc should back out of.
	vcnFilterName string

	// detailExport holds what "e" should export while in modeDetail — set
	// by "v" (security rules table), cleared whenever Enter opens the
	// plain YAML detail view instead, since that one has nothing sensible
	// to export as CSV.
	detailExport *detailExportData

	// showHelp toggles the LazyVim-style which-key popup (space bar). Not
	// a mode: other keys keep working normally while it's shown (and
	// close it after acting), so it's just an overlay flag checked at
	// render time, not something the Update dispatch branches on.
	showHelp bool

	sidebarHidden bool

	width, height int

	// tableHeight is the *logical* height last passed to table.WithHeight/
	// SetHeight — NOT the same as m.table.Height(), which returns the
	// viewport height AFTER bubbles subtracts the header row's height.
	// refreshTable rebuilds the table via newTable(), which itself calls
	// WithHeight — feeding it m.table.Height() back in would subtract the
	// header height a second time, shrinking the table by one line on
	// every single refresh (every filter keystroke calls refreshTable, so
	// this was visibly eating the table a line at a time). Keeping the
	// pre-subtraction value here is what makes that round-trip safe.
	tableHeight int
}

func New(factory *clients.Factory, scope registry.Scope, writeEnabled bool, profile string) Model {
	fi := textinput.New()
	fi.Placeholder = "filter..."

	ci := textinput.New()
	ci.Placeholder = "type resource name to confirm"

	pi := textinput.New()
	pi.Placeholder = "opc"

	return Model{
		factory:      factory,
		profile:      profile,
		resources:    registry.All(factory),
		scope:        scope,
		compPath:     []crumb{{ID: scope.CompartmentID, Name: "root"}},
		table:        newTable(20),
		detail:       viewport.New(80, 20),
		filterInput:  fi,
		confirmInput: ci,
		promptInput:  pi,
		writeEnabled: writeEnabled,
		loading:      true,
		autoRedirect: true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.fetchRootName())
}

func (m Model) current() registry.Resource {
	return m.resources[m.resIdx]
}

func (m Model) load() tea.Cmd {
	res := m.current()
	scope := m.scope
	return func() tea.Msg {
		rows, err := fetchAll(context.Background(), res, scope)
		return rowsMsg{rows: rows, err: err}
	}
}

func (m Model) fetchRootName() tea.Cmd {
	factory := m.factory
	region := m.scope.Region
	tenancyID := m.compPath[0].ID
	return func() tea.Msg {
		return rootNameMsg{name: rootCompartmentName(context.Background(), factory, region, tenancyID)}
	}
}

func (m Model) fetchRegions() tea.Cmd {
	factory := m.factory
	region := m.scope.Region
	tenancyID := m.compPath[0].ID
	return func() tea.Msg {
		items, err := listRegions(context.Background(), factory, region, tenancyID)
		return regionsMsg{items: items, err: err}
	}
}

func (m Model) fetchBastions() tea.Cmd {
	factory := m.factory
	scope := m.scope
	return func() tea.Msg {
		items, err := listBastions(context.Background(), factory, scope)
		return bastionsMsg{items: items, err: err}
	}
}

// createSession resolves the instance's private IP, finds a local SSH key,
// creates the bastion session and polls it to ACTIVE, then hands back a
// ready-to-run ssh command. It's a single blocking tea.Cmd — polling here
// doesn't block the UI since bubbletea runs each Cmd in its own goroutine.
func (m Model) createSession(bastionID string, row registry.Row, username string) tea.Cmd {
	factory := m.factory
	scope := m.scope
	return func() tea.Msg {
		ctx := context.Background()

		pubKey, privKeyPath, err := localSSHKeyPair()
		if err != nil {
			return sessionReadyMsg{err: err}
		}
		privateIP, err := instancePrivateIP(ctx, factory, scope, row.ID)
		if err != nil {
			return sessionReadyMsg{err: fmt.Errorf("resolve instance private IP: %w", err)}
		}
		session, err := createBastionSession(ctx, factory, scope, bastionID, row.ID, privateIP, username, pubKey)
		if err != nil {
			return sessionReadyMsg{err: fmt.Errorf("create bastion session: %w", err)}
		}
		sshCmd, err := buildSSHCommand(session, privKeyPath)
		if err != nil {
			return sessionReadyMsg{err: err}
		}
		return sessionReadyMsg{sshCmd: sshCmd}
	}
}

// newTable builds a fresh table.Model. Row-set swaps rebuild rather than
// mutate the existing table: bubbles' table/viewport pair tracks cursor and
// scroll offset internally, and swapping in a shorter row set while that
// state is stale panics (slice bounds) inside its own viewport math.
func newTable(height int) table.Model {
	if height <= 0 {
		height = 20
	}
	t := table.New(
		table.WithFocused(true),
		table.WithHeight(height),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.Bold(true).Foreground(lipgloss.Color("39"))
	s.Selected = selStyle
	t.SetStyles(s)
	return t
}

func applyFilter(rows []registry.Row, query string) []registry.Row {
	if query == "" {
		return rows
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name
	}
	matches := fuzzy.Find(query, names)
	out := make([]registry.Row, len(matches))
	for i, mm := range matches {
		out[i] = rows[mm.Index]
	}
	return out
}

// fitColumnWidth shrinks a column to whatever its header and the loaded
// rows actually need, up to the resource's declared Width — a ceiling, not
// a fixed size. Without this every column reserves its full declared width
// even when actual values are far shorter (e.g. IP(PUB/PRI) declares room
// for two full IPv4 addresses, but most rows show far less), which both
// wastes space and, with enough wide columns, can push later ones past the
// terminal's width entirely.
func fitColumnWidth(header string, values []string, ceiling int) int {
	w := len([]rune(header))
	for _, v := range values {
		if l := len([]rune(v)); l > w {
			w = l
		}
	}
	if w > ceiling {
		w = ceiling
	}
	return w
}

// tableColMinWidth is the floor a column can shrink to under proportional
// scaling — small enough to still show a few characters, not so small a
// column collapses to nothing readable.
const tableColMinWidth = 3

// fitColumns computes each column's content-fit width (fitColumnWidth), then,
// if their combined on-screen size (content + the 2-col Cell/Header padding
// bubbles adds per column) would exceed the available viewport width,
// scales every column down proportionally to fit — rather than leaving
// them at full size and letting bubbles silently crop whatever falls past
// the right edge (which is what used to happen to trailing columns on a
// narrower terminal or sidebar).
func fitColumns(cols []registry.Column, colValues [][]string, available int) []int {
	natural := make([]int, len(cols))
	total := 0
	for i, c := range cols {
		natural[i] = fitColumnWidth(c.Header, colValues[i], c.Width)
		total += natural[i] + 2
	}
	if available <= 0 || total <= available {
		return natural
	}

	padding := 2 * len(cols)
	contentAvailable := available - padding
	if contentAvailable < tableColMinWidth*len(cols) {
		contentAvailable = tableColMinWidth * len(cols)
	}
	contentTotal := total - padding
	scale := float64(contentAvailable) / float64(contentTotal)

	widths := make([]int, len(cols))
	for i, w := range natural {
		nw := int(float64(w) * scale)
		if nw < tableColMinWidth {
			nw = tableColMinWidth
		}
		widths[i] = nw
	}
	return widths
}

// refreshTable rebuilds m.table from the resource's current columns and the
// given rows, preserving the table's on-screen size.
func (m *Model) refreshTable(rows []registry.Row) {
	cols := m.current().Columns()
	trows := make([]table.Row, len(rows))
	colValues := make([][]string, len(cols))
	for i, row := range rows {
		cells := make([]string, len(cols))
		for j, c := range cols {
			cells[j] = c.Get(row)
			colValues[j] = append(colValues[j], cells[j])
		}
		trows[i] = cells
	}

	width := m.table.Width()
	widths := fitColumns(cols, colValues, width)
	tcols := make([]table.Column, len(cols))
	for i, c := range cols {
		tcols[i] = table.Column{Title: c.Header, Width: widths[i]}
	}

	t := newTable(m.tableHeight)
	if width > 0 {
		t.SetWidth(width)
	}
	t.SetColumns(tcols)
	t.SetRows(trows)
	m.table = t
}

func (m *Model) setDisplayRows() {
	m.displayRows = applyFilter(m.rows, m.filterQuery)
	m.refreshTable(m.displayRows)
}

// mainAbsFloor is the main panel's true minimum, distinct from mainMinWidth
// (sidebar.go) — that constant is only the *preferred* reservation used
// when deciding how far to shrink the sidebar. Flooring here at that same
// preferred value too would let the two sides' floors add up to more than
// a narrow terminal actually has (sidebarContentWidth already gives up on
// the full reservation and shrinks itself once the terminal's that tight —
// see sidebarAbsFloor).
const mainAbsFloor = 10

// mainContentWidth is how many columns the main panel (table/detail/status
// line) has to work with, to the right of the sidebar.
func (m Model) mainContentWidth() int {
	w := m.width - sidebarTotalWidth(m)
	if !m.sidebarHidden {
		w -= 2 // gutter between sidebar and main content in View()
	}
	if w < mainAbsFloor {
		w = mainAbsFloor
	}
	return w
}

// relayout recomputes the table/detail width against the sidebar's current
// on-screen width. Needed on every window resize, and also whenever the
// sidebar's own content width could have changed — a compartment path
// growing/shrinking, or the sidebar being toggled — since none of those are
// resize events.
func (m *Model) relayout() {
	mainWidth := m.mainContentWidth()
	m.table.SetWidth(mainWidth)
	m.detail.Width = mainWidth
}

func (m *Model) switchResource(idx int) tea.Cmd {
	m.resIdx = idx
	m.rows = nil
	m.filterQuery = ""
	m.loading = true
	m.err = nil
	m.autoRedirect = false
	// A VCN filter stays active while moving between VCN-scoped resources
	// (Subnets, Instances, ...) so the sidebar tree can be used to hop
	// between them without re-picking the VCN. Switching to anything else
	// (Compartments, DRGs, or back to the VCN list itself) drops it.
	if !isVcnDependent(m.resources[idx].Key()) {
		m.scope.VcnID = ""
		m.vcnFilterName = ""
	}
	m.setDisplayRows()
	return m.load()
}

// selectVcnFilter scopes every VCN-dependent resource (see
// resourceCategories) to one VCN — triggered by "i" on a VCN row — then
// opens the sidebar so the user can pick which of them to look at. It
// doesn't switch resource or reload itself: nothing needs fetching until a
// specific resource is picked from the tree.
func (m *Model) selectVcnFilter(id, name string) {
	m.scope.VcnID = id
	m.vcnFilterName = name
	m.openSidebar()
}

// exitVcn clears the VCN scope and returns to the VCN list.
func (m *Model) exitVcn() tea.Cmd {
	idx := -1
	for i, r := range m.resources {
		if r.Key() == "vcn" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	m.scope.VcnID = ""
	m.vcnFilterName = ""
	return m.switchResource(idx)
}

// openSidebar switches focus into the sidebar tree, positioned on the
// currently active resource. Un-hides it first if it was toggled off —
// there's no point focusing a tree the user can't see.
func (m *Model) openSidebar() {
	if m.sidebarHidden {
		m.sidebarHidden = false
		m.relayout()
	}
	leaves := flatLeaves(buildSidebar(m.resources))
	for i, l := range leaves {
		if l.resIdx == m.resIdx {
			m.sidebarCursor = i
			break
		}
	}
	m.mode = modeSidebar
}

// switchToRootCompartments jumps back to the tenancy root and shows its
// compartment list. Used when the sidebar's Compartments leaf is picked: the
// current scope is usually already deep inside a leaf compartment (that's
// how we got redirected to VCNs in the first place), so reloading
// Compartments at the current scope would just show another empty table.
// Starting over from the root lets the user drill back down from scratch.
func (m *Model) switchToRootCompartments() tea.Cmd {
	idx := -1
	for i, r := range m.resources {
		if r.Key() == "compartment" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	m.compPath = m.compPath[:1]
	m.scope.CompartmentID = m.compPath[0].ID
	m.relayout()
	return m.switchResource(idx)
}

func (m *Model) enterCompartment(id, name string) tea.Cmd {
	m.compPath = append(m.compPath, crumb{ID: id, Name: name})
	m.scope.CompartmentID = id
	m.rows = nil
	m.filterQuery = ""
	m.setDisplayRows()
	m.loading = true
	m.err = nil
	m.autoRedirect = true
	m.relayout()
	return m.load()
}

func (m *Model) exitCompartment() tea.Cmd {
	if len(m.compPath) <= 1 {
		return nil
	}
	m.compPath = m.compPath[:len(m.compPath)-1]
	m.scope.CompartmentID = m.compPath[len(m.compPath)-1].ID
	m.rows = nil
	m.filterQuery = ""
	m.setDisplayRows()
	m.loading = true
	m.err = nil
	m.relayout()
	return m.load()
}

func (m *Model) selected() (registry.Row, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.displayRows) {
		return registry.Row{}, false
	}
	return m.displayRows[i], true
}

func (m Model) actionable() (registry.Actionable, bool) {
	a, ok := m.current().(registry.Actionable)
	return a, ok
}

func (m Model) runAction(spec registry.ActionSpec, row registry.Row) tea.Cmd {
	a, ok := m.actionable()
	if !ok {
		return nil
	}
	scope := m.scope
	return func() tea.Msg {
		err := a.RunAction(context.Background(), scope, spec.Key, row.ID)
		return actionResultMsg{label: spec.Label + " " + row.Name, err: err}
	}
}

func renderDetail(row registry.Row) string {
	b, err := yaml.Marshal(row.Raw)
	if err != nil {
		return fmt.Sprintf("failed to render detail: %v", err)
	}
	return string(b)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.tableHeight = msg.Height - 8
		m.table.SetHeight(m.tableHeight)
		m.detail.Height = msg.Height - 6
		m.relayout()
		m.refreshTable(m.displayRows)
		return m, nil

	case rowsMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.rows = msg.rows
			m.setDisplayRows()
			// A compartment with no sub-compartments would otherwise leave
			// an empty table on screen; jump to VCNs instead — instances
			// are scoped to a VCN in this app (see "i" in updateTable), so
			// VCN is the more useful default landing resource. Only for
			// loads that just descended into a compartment (autoRedirect) —
			// not a manual switch back to Compartments, or this would just
			// bounce straight back to VCNs on an already-empty leaf.
			redirect := m.autoRedirect
			m.autoRedirect = false
			if redirect && len(msg.rows) == 0 && m.current().Key() == "compartment" {
				for idx, r := range m.resources {
					if r.Key() == "vcn" {
						return m, m.switchResource(idx)
					}
				}
			}
		}
		return m, nil

	case rootNameMsg:
		if len(m.compPath) > 0 {
			m.compPath[0].Name = msg.name
			m.relayout()
		}
		return m, nil

	case regionsMsg:
		if msg.err != nil {
			m.statusMsg = "region list failed: " + msg.err.Error()
			m.mode = modeTable
			return m, nil
		}
		m.regionItems = msg.items
		if m.mode == modePicker && m.picker.kind == pickerRegion {
			m.picker.items = msg.items
			m.picker.refilter()
		}
		return m, nil

	case actionResultMsg:
		if msg.err != nil {
			m.statusMsg = msg.label + " failed: " + msg.err.Error()
			return m, nil
		}
		m.statusMsg = msg.label + " requested"
		m.loading = true
		return m, m.load()

	case diagramMsg:
		if msg.err != nil {
			m.statusMsg = "diagram failed: " + msg.err.Error()
			return m, nil
		}
		m.statusMsg = "diagram written to " + msg.path
		return m, nil

	case bastionsMsg:
		if msg.err != nil {
			m.statusMsg = "list bastions failed: " + msg.err.Error()
			return m, nil
		}
		switch len(msg.items) {
		case 0:
			m.statusMsg = "no bastion found in this compartment"
		case 1:
			m.sshBastionID = msg.items[0].key
			m.promptInput.SetValue("opc")
			m.promptInput.CursorEnd()
			m.promptInput.Focus()
			m.mode = modePrompt
		default:
			m.picker = newPicker(pickerBastion, "bastion", msg.items)
			m.mode = modePicker
		}
		return m, nil

	case sessionReadyMsg:
		if msg.err != nil {
			m.statusMsg = "bastion session failed: " + msg.err.Error()
			return m, nil
		}
		m.statusMsg = ""
		c := exec.Command("sh", "-c", msg.sshCmd)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return sshDoneMsg{err: err}
		})

	case sshDoneMsg:
		if msg.err != nil {
			m.statusMsg = "ssh exited with error: " + msg.err.Error()
		} else {
			m.statusMsg = "ssh session closed"
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeDetail:
			return m.updateDetail(msg)
		case modePicker:
			return m.updatePicker(msg)
		case modeFilter:
			return m.updateFilter(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		case modePrompt:
			return m.updatePrompt(msg)
		case modeSidebar:
			return m.updateSidebar(msg)
		default:
			return m.updateTable(msg)
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeTable
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "e":
		if m.detailExport == nil {
			return m, nil
		}
		path := exportFilename(m.detailExport.filenameSuffix, time.Now())
		if err := writeCSVFile(path, m.detailExport.header, m.detailExport.records); err != nil {
			m.statusMsg = "export failed: " + err.Error()
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("exported %d rows to %s", len(m.detailExport.records), path)
		return m, nil
	}
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeTable
		return m, nil
	case "enter":
		item, ok := m.picker.selected()
		m.mode = modeTable
		if !ok {
			return m, nil
		}
		switch m.picker.kind {
		case pickerRegion:
			m.scope.Region = item.key
			m.rows = nil
			m.filterQuery = ""
			m.setDisplayRows()
			m.loading = true
			m.err = nil
			return m, m.load()
		case pickerAction:
			a, ok := m.actionable()
			if !ok {
				return m, nil
			}
			for _, spec := range a.Actions() {
				if spec.Key == item.key {
					m.pendingAction = spec
					m.confirmInput.SetValue("")
					m.confirmInput.Focus()
					m.mode = modeConfirm
					break
				}
			}
		case pickerBastion:
			m.sshBastionID = item.key
			m.promptInput.SetValue("opc")
			m.promptInput.CursorEnd()
			m.promptInput.Focus()
			m.mode = modePrompt
		}
		return m, nil
	case "up", "ctrl+k":
		if m.picker.cursor > 0 {
			m.picker.cursor--
		}
		return m, nil
	case "down", "ctrl+j":
		if m.picker.cursor < len(m.picker.filtered)-1 {
			m.picker.cursor++
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.picker.input, cmd = m.picker.input.Update(msg)
	m.picker.refilter()
	return m, cmd
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filterQuery = m.filterBak
		m.setDisplayRows()
		m.mode = modeTable
		return m, nil
	case "enter":
		m.filterQuery = m.filterInput.Value()
		m.mode = modeTable
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filterQuery = m.filterInput.Value()
	m.setDisplayRows()
	return m, cmd
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeTable
		return m, nil
	case "enter":
		m.mode = modeTable
		if m.confirmInput.Value() != m.pendingRow.Name {
			m.statusMsg = "name did not match — action cancelled"
			return m, nil
		}
		m.statusMsg = m.pendingAction.Label + " " + m.pendingRow.Name + "..."
		return m, m.runAction(m.pendingAction, m.pendingRow)
	}
	var cmd tea.Cmd
	m.confirmInput, cmd = m.confirmInput.Update(msg)
	return m, cmd
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeTable
		return m, nil
	case "enter":
		username := m.promptInput.Value()
		if username == "" {
			username = "opc"
		}
		m.mode = modeTable
		m.statusMsg = "creating bastion session for " + m.pendingRow.Name + "..."
		return m, m.createSession(m.sshBastionID, m.pendingRow, username)
	}
	var cmd tea.Cmd
	m.promptInput, cmd = m.promptInput.Update(msg)
	return m, cmd
}

func (m Model) updateTable(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Any key closes the shortcuts popup — like LazyVim's which-key,
	// pressing the actual shortcut both dismisses it and runs the action.
	// Esc just closes it without also triggering Esc's usual "go up" —
	// pressing it twice gets you there.
	if m.showHelp {
		m.showHelp = false
		if key == "esc" {
			return m, nil
		}
	}

	switch key {
	case " ":
		m.showHelp = !m.showHelp
		return m, nil

	case "ctrl+c", "q":
		return m, tea.Quit

	case ":":
		m.openSidebar()
		return m, nil

	case "t":
		m.sidebarHidden = !m.sidebarHidden
		m.relayout()
		return m, nil

	case "r":
		m.picker = newPicker(pickerRegion, "region", m.regionItems)
		m.mode = modePicker
		if len(m.regionItems) > 0 {
			return m, nil
		}
		return m, m.fetchRegions()

	case "R":
		m.loading = true
		m.err = nil
		return m, m.load()

	case "e":
		path := exportFilename(m.current().Key(), time.Now())
		if err := exportCSV(path, m.current().Columns(), m.displayRows); err != nil {
			m.statusMsg = "export failed: " + err.Error()
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("exported %d rows to %s", len(m.displayRows), path)
		return m, nil

	case "/":
		m.filterBak = m.filterQuery
		m.filterInput.SetValue(m.filterQuery)
		m.filterInput.CursorEnd()
		m.filterInput.Focus()
		m.mode = modeFilter
		return m, nil

	case "tab":
		return m, m.switchResource((m.resIdx + 1) % len(m.resources))

	case "a":
		if !m.writeEnabled {
			m.statusMsg = "actions disabled (readonly mode; pass --write to enable)"
			return m, nil
		}
		a, ok := m.actionable()
		if !ok {
			return m, nil
		}
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		specs := a.Actions()
		items := make([]pickerItem, len(specs))
		for i, spec := range specs {
			items[i] = pickerItem{key: spec.Key, label: spec.Label}
		}
		m.pendingRow = row
		m.picker = newPicker(pickerAction, "action: "+row.Name, items)
		m.mode = modePicker
		return m, nil

	case "s":
		if !m.writeEnabled {
			m.statusMsg = "ssh disabled (readonly mode; pass --write to enable)"
			return m, nil
		}
		if m.current().Key() != "instance" {
			return m, nil
		}
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.pendingRow = row
		m.statusMsg = "looking for a bastion..."
		return m, m.fetchBastions()

	case "i":
		if m.current().Key() != "vcn" {
			return m, nil
		}
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.selectVcnFilter(row.ID, row.Name)
		return m, nil

	case "v":
		if m.current().Key() != "security-list" {
			return m, nil
		}
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		content, records, name, ok := securityRulesView(row)
		if !ok {
			return m, nil
		}
		m.mode = modeDetail
		m.detail.SetContent(content)
		m.detail.GotoTop()
		m.detailExport = &detailExportData{
			filenameSuffix: "security-rules-" + name,
			header:         securityRuleHeaders,
			records:        records,
		}
		return m, nil

	case "m":
		if m.vcnFilterName == "" {
			m.statusMsg = "pick a VCN first (\"i\" on a VCN row) to build its diagram"
			return m, nil
		}
		m.statusMsg = "building diagram..."
		return m, m.buildVcnDiagram()

	case "enter":
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		if m.current().Key() == "compartment" {
			return m, m.enterCompartment(row.ID, row.Name)
		}
		m.mode = modeDetail
		m.detail.SetContent(renderDetail(row))
		m.detail.GotoTop()
		m.detailExport = nil
		return m, nil

	case "esc":
		if m.filterQuery != "" {
			m.filterQuery = ""
			m.setDisplayRows()
			return m, nil
		}
		if m.scope.VcnID != "" {
			return m, m.exitVcn()
		}
		if cmd := m.exitCompartment(); cmd != nil {
			return m, cmd
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(pathStyle.Render("Profile:  "))
	b.WriteString(titleStyle.Render(m.profile))
	b.WriteString("\n")
	b.WriteString(pathStyle.Render("Region:   "))
	b.WriteString(titleStyle.Render(m.scope.Region))
	b.WriteString("\n")
	b.WriteString(pathStyle.Render("Resource: "))
	b.WriteString(titleStyle.Render(m.current().Label()))
	path := breadcrumbLabel(m.compPath)
	if m.vcnFilterName != "" {
		path += " › " + m.vcnFilterName
	}
	if path != "" {
		b.WriteString("    ")
		b.WriteString(pathStyle.Render(path))
	}
	b.WriteString("\n\n")

	var main strings.Builder
	switch m.mode {
	case modeDetail:
		main.WriteString(m.detail.View())
		main.WriteString("\n")
		hint := "esc: back"
		if m.detailExport != nil {
			hint += " · e: export csv"
		}
		rendered := statusStyle.Render(hint)
		if m.statusMsg != "" {
			rendered += statusStyle.Render(" · ") + renderStatusMsg(m.statusMsg)
		}
		main.WriteString(rendered)

	case modePicker:
		main.WriteString(m.renderPicker())

	case modeConfirm:
		main.WriteString(m.renderConfirm())

	case modePrompt:
		main.WriteString(m.renderPrompt())

	default:
		switch {
		case m.err != nil:
			main.WriteString(errorStyle.Render("error: " + m.err.Error()))
		case m.loading:
			main.WriteString(statusStyle.Render("loading..."))
		default:
			tableView := m.table.View()
			if m.current().Key() == "instance" {
				tableView = colorizeInstanceState(tableView, m.table.Columns())
			}
			main.WriteString(tableView)
		}
		main.WriteString("\n")
		// MaxWidth clips rather than lets the terminal soft-wrap: an
		// unclipped status line (long filter query + all the key hints)
		// can exceed terminal width, and bubbletea's own line-count
		// bookkeeping doesn't know about the terminal's wrap — every
		// redraw after that was off by the wrapped line count, which
		// showed up as the whole screen creeping upward while filtering.
		clip := lipgloss.NewStyle().MaxWidth(m.mainContentWidth())
		main.WriteString(clip.Render(m.renderStatusLine()))

		if m.mode == modeFilter {
			main.WriteString("\n")
			main.WriteString(clip.Render("/" + m.filterInput.View()))
		}
	}

	if m.sidebarHidden {
		b.WriteString(main.String())
	} else {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, m.renderSidebar(), "  ", main.String()))
	}

	out := b.String()
	if m.showHelp {
		out = overlayBottomRight(out, renderHelpBox(m), m.width)
	}
	return out
}

func (m Model) renderPicker() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.picker.title))
	b.WriteString("\n> ")
	b.WriteString(m.picker.input.View())
	b.WriteString("\n\n")
	for i, it := range m.picker.filtered {
		line := it.label
		if i == m.picker.cursor {
			line = selStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(m.picker.filtered) == 0 {
		b.WriteString(statusStyle.Render("  (no matches)"))
	}
	return boxStyle.Render(b.String())
}

func (m Model) renderConfirm() string {
	var b strings.Builder
	b.WriteString(errorStyle.Render(fmt.Sprintf("%s %q", m.pendingAction.Label, m.pendingRow.Name)))
	b.WriteString("\n\ntype the resource name to confirm:\n\n> ")
	b.WriteString(m.confirmInput.View())
	b.WriteString("\n\n")
	b.WriteString(statusStyle.Render("enter: confirm · esc: cancel"))
	return boxStyle.Render(b.String())
}

func (m Model) renderPrompt() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("SSH via Bastion: " + m.pendingRow.Name))
	b.WriteString("\n\nOS username on target:\n\n> ")
	b.WriteString(m.promptInput.View())
	b.WriteString("\n\n")
	b.WriteString(statusStyle.Render("enter: confirm · esc: cancel"))
	return boxStyle.Render(b.String())
}

// helpEntries lists every keybinding that applies to the current state —
// shared by the space-bar shortcuts popup (help.go) and used to be the
// entire content of the status line before it moved there. Order here is
// the order both render in.
func (m Model) helpEntries() []helpEntry {
	var entries []helpEntry
	add := func(key, desc string) { entries = append(entries, helpEntry{key, desc}) }

	add("j/k, ↑↓", "move")
	add("enter", "select")
	add(":", "resource tree")
	add("/", "filter")
	add("r", "region")
	add("R", "refresh")
	add("e", "export csv")
	if m.vcnFilterName != "" {
		add("m", "export diagram")
	}
	if m.sidebarHidden {
		add("t", "show tree")
	} else {
		add("t", "hide tree")
	}
	if _, ok := m.actionable(); ok {
		if m.writeEnabled {
			add("a", "actions")
		} else {
			add("a", "actions (readonly)")
		}
	}
	if m.current().Key() == "instance" {
		if m.writeEnabled {
			add("s", "ssh")
		} else {
			add("s", "ssh (readonly)")
		}
	}
	if m.current().Key() == "vcn" {
		add("i", "filter by this VCN")
	}
	if m.current().Key() == "security-list" {
		add("v", "view rules")
	}
	if len(m.compPath) > 1 || m.vcnFilterName != "" {
		add("esc", "up")
	}
	add("q", "quit")
	return entries
}

// renderStatusMsg colors m.statusMsg green for a successful export, red
// for a failure (every failure message in this app says "failed" or
// "error" — see the m.statusMsg assignments in Update()), and leaves
// plain info messages the usual dim status color.
func renderStatusMsg(msg string) string {
	switch {
	case strings.HasPrefix(msg, "exported ") || strings.HasPrefix(msg, "diagram written"):
		return successStyle.Render(msg)
	case strings.Contains(msg, "failed") || strings.Contains(msg, "error"):
		return errorStyle.Render(msg)
	default:
		return statusStyle.Render(msg)
	}
}

// renderStatusLine is deliberately terse now — the full shortcut list
// lives in the space-bar popup (help.go) instead of a single hard-to-scan
// gray line spanning the terminal on every screen.
func (m Model) renderStatusLine() string {
	parts := []string{fmt.Sprintf("%d items", len(m.displayRows))}
	if m.filterQuery != "" {
		parts = append(parts, fmt.Sprintf("filter: %q", m.filterQuery))
	}
	parts = append(parts, "space: shortcuts")
	line := statusStyle.Render(strings.Join(parts, " · "))
	if m.statusMsg != "" {
		line += statusStyle.Render(" · ") + renderStatusMsg(m.statusMsg)
	}
	return line
}
