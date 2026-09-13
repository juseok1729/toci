// Package app holds the bubbletea model: state machine and view for toci.
package app

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	oci_bastion "github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/sahilm/fuzzy"
	"gopkg.in/yaml.v3"

	"toci/internal/clients"
	"toci/internal/registry"
)

// The accent/chrome palette below is sampled straight from a screenshot of
// the OCI console's own sidebar (a thin decorative strip, not a flat brand
// color) — 7 swatches ranked by pixel share; see docs/COLOR_SYSTEM.md.
// Semantic colors (success/error/state badges in state_color.go) are left
// out on purpose: those signal meaning (red = stopped/error), and
// retheming them to green would make that signal ambiguous. splash.go has
// its own copies of the old statusStyle/pathStyle values (splashMutedStyle/
// splashProfileStyle) so the splash screen's look doesn't move with this.
const (
	ociAccent = "#689878" // dominant sage, 29% of the sampled strip
	ociBorder = "#487858" // mid forest — structural chrome (borders)
	ociMuted  = "#588868" // unused for now — a dimmer status green, kept catalogued
	ociSubtle = "#88b898" // lightest — status text, secondary/breadcrumb text
	ociSelBg  = "#386848" // dark forest — selected-row background
	ociHighlt = "#e8c878" // the beige/gold accent blob in the strip
)

var (
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(ociSubtle))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	titleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(ociAccent)).Bold(true)
	pathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(ociSubtle))
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(ociBorder)).Padding(0, 1)
	selStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color(ociSelBg)).Bold(true)

	// headerValueStyle is titleStyle's white counterpart, scoped to just the
	// Profile/Region/Resource/Compartment values and the corner version
	// line — titleStyle itself stays ociAccent everywhere else (picker/box
	// titles), so this only touches what was asked.
	headerValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
)

type mode int

const (
	modeTable mode = iota
	modeDetail
	modePicker
	modeFilter
	modeConfirm
	modePrompt
	modeSplash
	modeEmbeddedTerm
)

type rowsMsg struct {
	rows []registry.Row
	err  error
}

type rootNameMsg struct{ name string }

// vcnNamesMsg carries the VcnID->vcnInfo lookup fetchVcnNames builds, for
// labeling the "g" grouping header on the Subnet view (name, CIDR, IP
// range).
type vcnNamesMsg struct {
	names map[string]vcnInfo
	err   error
}

type regionsMsg struct {
	items []pickerItem
	err   error
}

// recentRowWindow is the "created within" threshold that makes a row
// eligible to blink.
const recentRowWindow = 3 * 24 * time.Hour

type actionResultMsg struct {
	label string
	msg   string
	err   error
}

type bastionsMsg struct {
	items []pickerItem
	err   error
}

type sessionReadyMsg struct {
	sshCmd string
	err    error

	// cacheKey/expiresAt are set for any bastion session (fresh or reused;
	// empty only for a direct session, which has no server-side session to
	// cache) — see Model.bastionSessions.
	cacheKey  string
	expiresAt time.Time
	// reused is true when sshCmd came from an existing session (memory
	// cache or findReusableSession) rather than a brand-new CreateSession.
	// A reused session was registered with whatever key was active when it
	// was first created — if the key picked for *this* attempt differs,
	// authentication fails even though the session itself is healthy; see
	// embTermExitMsg's quickFail retry.
	reused bool
}

// cachedBastionSession is one entry in Model.bastionSessions: a bastion
// session's already-built ssh command, reusable until the session's own
// TTL runs out.
type cachedBastionSession struct {
	sshCmd    string
	expiresAt time.Time
}

// bastionSessionReuseMargin is the headroom required before a cached
// session's expiresAt for it to still be handed out — avoids starting a
// connection on a session that OCI is about to expire out from under it.
const bastionSessionReuseMargin = 2 * time.Minute

type Model struct {
	factory   *clients.Factory
	profile   string
	version   string
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
	sshDirect    bool
	promptInput  textinput.Model
	embTerm      *embeddedTerm

	// sshKeyPairs/sshKey hold the local-key resolution step: sshKeyPairs is
	// the candidate list shown in the pickerSSHKey picker when more than one
	// was found under ~/.ssh; sshKey is the one resolved (picked, or the
	// sole candidate auto-selected) for the ssh setup in progress.
	sshKeyPairs []sshKeyPair
	sshKey      sshKeyPair
	// sshUsername is the OS username from the last modePrompt submission —
	// kept around (alongside sshBastionID/pendingRow/sshKey) so
	// embTermExitMsg's automatic retry can re-issue the same connection
	// with skipReuse, without re-asking the user for anything.
	sshUsername string

	// bastionSessions caches a live bastion session's ssh command by
	// bastionSessionCacheKey, so reconnecting to the same target+user
	// within the session's TTL reuses it instead of creating a new one
	// (each bastion allows only a few concurrent sessions). Cleared per
	// entry on its own TTL (checked at lookup time) or when a reused
	// session turns out to be dead (see embTermExitMsg).
	bastionSessions map[string]cachedBastionSession
	// activeSessionCacheKey is the cache key for the bastion session the
	// current embedded terminal is using, "" for a direct session — lets
	// embTermExitMsg evict a session that just failed instead of handing
	// out the same broken one again until it naturally expires.
	activeSessionCacheKey string
	// activeSessionWasReused mirrors sessionReadyMsg.reused for the session
	// the current embedded terminal is using — embTermExitMsg only retries
	// with skipReuse when this is true and the failure was a quickFail;
	// a fresh session that fails quickly is a real problem (bad key,
	// unreachable network), not a stale-reuse mismatch, and retrying it
	// would just repeat the same failure.
	activeSessionWasReused bool

	mode      mode
	loading   bool
	err       error
	statusMsg string

	// compTree is the cached full compartment hierarchy (see
	// compartment_tree.go), fetched once at startup — the "c" picker (F5),
	// header breadcrumb, and subtree fan-out (F3) all read from this
	// instead of re-listing compartments live.
	compTree *compartmentTree
	// tenancyName is the tenancy's resolved display name (rootNameMsg) —
	// kept separately so it can be reapplied to compTree's root node
	// whichever of the two async loads (root name, tree) finishes last.
	tenancyName string

	// recentList is the MRU compartment list behind the header's "Recent:"
	// line and its "1".."9" hotkeys (F2) — loaded from disk in New(),
	// persisted on every compartment switch.
	recentList []recentEntry

	// restoreCursorID is the previously-selected row's ID, set right
	// before a compartment switch reload — the next rowsMsg tries to put
	// the cursor back on the row with this ID (F1: "동일 리소스 ID가 있으면
	// 커서 복원"), then clears it either way.
	restoreCursorID string

	// subtreeOn is the "C" toggle (F3): fetch the current compartment plus
	// every descendant instead of just the one. subtreeGen/subtreeCancel
	// guard/cancel the fan-out this kicks off (see subtree.go);
	// subtreeTargets/subtreeRows/subtreeDone track its progress;
	// subtreeSkipped counts compartments skipped for 401/404,
	// subtreeErrMsg the last non-skip error.
	subtreeOn      bool
	subtreeGen     int
	subtreeCancel  context.CancelFunc
	subtreeTargets []subtreeTarget
	subtreeRows    map[string][]registry.Row
	subtreeDone    map[string]bool
	subtreeSkipped int
	subtreeErrMsg  string

	// vcnFilterName is non-empty while the Instance table is scoped to one
	// VCN (scope.VcnID set) via the "i" key on a VCN row — shown in the
	// header and used to know what Esc should back out of.
	vcnFilterName string

	// drgFilterName is the DRG-attachment analog of vcnFilterName — non-empty
	// while scope.DrgID is set via "i"/Enter on a DRG row.
	drgFilterName string

	// detailExport holds what "e" should export while in modeDetail — set
	// by "v" (security rules table), cleared whenever Enter opens the
	// plain YAML detail view instead, since that one has nothing sensible
	// to export as CSV.
	detailExport *detailExportData

	// resourceMap is non-nil while modeDetail is showing the "M" resource
	// map — updateDetail checks it to route j/k to
	// resourceMapSelected (which subnet's path through to its gateways is
	// highlighted) instead of the viewport's own scrolling, and re-renders
	// on every move. Cleared on leaving modeDetail so those keys go back
	// to scrolling for every other kind of detail content.
	resourceMap         *resourceMapData
	resourceMapSelected int

	// showHelp toggles the LazyVim-style which-key popup (space bar). Not
	// a mode: other keys keep working normally while it's shown (and
	// close it after acting), so it's just an overlay flag checked at
	// render time, not something the Update dispatch branches on.
	showHelp bool

	// splashProgress/splashFrame drive the startup splash screen's fake
	// progress bar and spinner — "fake" because the only real signal is
	// fetchRootName's GetCompartment call finishing (there's no initial
	// resource load to wait on — see Init()'s own doc); a bar that jumps
	// through a couple of stages (see splashStages/splashStageTicks in
	// splash.go) reads as alive instead of stalling on an indeterminate
	// wait. splashProgress is held at the second-to-last stage until
	// splashDataReady (the real fetch finished), so on a fast connection
	// the splash still holds for a minimum ~1.2s instead of flashing by in
	// whatever the API round-trip happened to take.
	splashProgress  int
	splashFrame     int
	splashDataReady bool

	// splashPhrase is a splashPhrases entry, re-rolled each time
	// splashProgress advances to a new stage rather than per-tick — a
	// phrase that changes every 60ms just reads as flickering.
	// splashSpinnerFrame advances in lockstep with it (see splashTickMsg),
	// matching taws's SplashState::set_message, which bumps its spinner
	// frame only when the status message changes.
	splashPhrase       string
	splashSpinnerFrame int

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

	// blinkOn alternates on blinkTickCmd's timer to drive blinkRecentRows.
	blinkOn bool

	// bastionPending is true while a bastion session lookup/creation Cmd
	// (from createSession — the leg that can block on OCI up to ~90s) is in
	// flight, driving the status-bar spinner via bastionSpinnerTickCmd.
	// Unlike blinkTickCmd (started once in Init, runs forever), this tick
	// chain is started only for the wait and stops itself the moment this
	// flips back to false, since it's naturally bounded by that one Cmd.
	bastionPending      bool
	bastionSpinnerFrame int
	// blinkEnabled is the user-facing on/off switch for the whole feature
	// ("b" key) — some users just don't want blinking rows, independent of
	// blinkOn's animation timer.
	blinkEnabled bool

	// groupByVcn toggles ("g" key) grouping the Subnet list by VCN — only
	// meaningful on the Subnet view with no VCN filter active (see
	// groupingActive); a VCN-filtered Subnet list is already one VCN.
	groupByVcn bool
	// vcnNames caches VcnID->vcnInfo (name, CIDR) for the current
	// compartment, fetched lazily by fetchVcnNames the first time
	// groupByVcn turns on. Invalidated (set nil) on every compartment
	// change.
	vcnNames map[string]vcnInfo

	// exascaleNodeTree toggles ("g" key) expanding each Exadata VM cluster
	// (Exascale) row into a tree with its DB nodes as children — the node
	// data already rides along on ExadbVmClusterRow.Nodes from List, so
	// unlike groupByVcn this needs no extra fetch/cache.
	exascaleNodeTree bool
}

func New(factory *clients.Factory, scope registry.Scope, writeEnabled bool, profile, version string) Model {
	fi := textinput.New()
	fi.Placeholder = "filter..."
	fi.Prompt = "" // renderer draws its own "/" prefix

	ci := textinput.New()
	ci.Placeholder = "type resource name to confirm"
	ci.Prompt = "" // renderConfirm draws its own "> " prefix

	pi := textinput.New()
	pi.Placeholder = "opc"
	pi.Prompt = "" // renderPrompt draws its own "> " prefix

	return Model{
		factory:         factory,
		profile:         profile,
		version:         version,
		resources:       registry.All(factory),
		scope:           scope,
		compPath:        []crumb{{ID: scope.CompartmentID, Name: "root"}},
		table:           newTable(20),
		detail:          viewport.New(viewport.WithWidth(80), viewport.WithHeight(20)),
		filterInput:     fi,
		confirmInput:    ci,
		promptInput:     pi,
		writeEnabled:    writeEnabled,
		mode:            modeSplash,
		splashPhrase:    splashPhrases[rand.Intn(len(splashPhrases))],
		blinkEnabled:    true,
		bastionSessions: map[string]cachedBastionSession{},
		recentList:      loadRecent(profile),
	}
}

// Init deliberately doesn't load anything into the table (no m.load()) —
// like k9s/taws, the first screen is an empty table with the resource
// search open (see splashTickMsg), not whatever resIdx happens to default
// to. fetchRootName still runs up front since the header's "Compartment:"
// line needs it regardless of which resource gets picked first.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchRootName(), m.fetchCompartmentTreeCmd(), splashTickCmd(), blinkTickCmd())
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

type blinkTickMsg struct{}

func blinkTickCmd() tea.Cmd {
	return tea.Tick(600*time.Millisecond, func(time.Time) tea.Msg {
		return blinkTickMsg{}
	})
}

type bastionSpinnerTickMsg struct{}

func bastionSpinnerTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return bastionSpinnerTickMsg{}
	})
}

func (m Model) fetchBastions() tea.Cmd {
	factory := m.factory
	scope := m.scope
	return func() tea.Msg {
		items, err := listBastions(context.Background(), factory, scope)
		return bastionsMsg{items: items, err: err}
	}
}

// createSession resolves the instance's private IP, then either reuses an
// already-ACTIVE session found live on OCI (see findReusableSession — this
// is what makes reuse survive a toci restart, unlike the in-memory-only
// Model.bastionSessions checked before this Cmd even runs) or creates a
// fresh bastion session with the already-resolved local key and polls it
// to ACTIVE, then hands back a ready-to-run ssh command plus the cache
// entry for it. It's a single blocking tea.Cmd — polling here doesn't
// block the UI since bubbletea runs each Cmd in its own goroutine.
//
// skipReuse forces a fresh session even if a reusable one would otherwise
// be found — used for the automatic retry after a reused session's key
// turns out not to match it (see embTermExitMsg's quickFail handling): a
// second lookup would just find the same session again and fail the same
// way.
func (m Model) createSession(bastionID string, row registry.Row, username string, key sshKeyPair, skipReuse bool) tea.Cmd {
	if node, ok := row.Raw.(exascaleNodeRow); ok {
		return m.createExascaleNodeSession(bastionID, row, node, username, key, skipReuse)
	}
	factory := m.factory
	scope := m.scope
	return func() tea.Msg {
		ctx := context.Background()

		privateIP, err := instancePrivateIP(ctx, factory, scope, row.ID)
		if err != nil {
			return sessionReadyMsg{err: fmt.Errorf("resolve instance private IP: %w", err)}
		}

		cacheKey := bastionSessionCacheKey(bastionID, row.ID, username)
		if !skipReuse {
			if existing, expiresAt, ok := findReusableSession(ctx, factory, scope, bastionID, row.ID, username); ok {
				if sshCmd, err := buildSSHCommand(existing, key.privateKeyPath); err == nil {
					return sessionReadyMsg{sshCmd: sshCmd, cacheKey: cacheKey, expiresAt: expiresAt, reused: true}
				}
			}
		}

		createdAt := time.Now()
		session, err := createBastionSession(ctx, factory, scope, bastionID, row.ID, privateIP, username, key.pubKeyContent)
		if err != nil {
			return sessionReadyMsg{err: fmt.Errorf("create bastion session: %w", err)}
		}
		sshCmd, err := buildSSHCommand(session, key.privateKeyPath)
		if err != nil {
			return sessionReadyMsg{err: err}
		}
		return sessionReadyMsg{
			sshCmd:    sshCmd,
			cacheKey:  cacheKey,
			expiresAt: createdAt.Add(bastionSessionTTLSeconds * time.Second),
		}
	}
}

// createExascaleNodeSession is createSession's Exascale counterpart. OCI
// Bastion has no notion of an Exadata VM Cluster as a session target
// (confirmed live — CreateSession 404s "NotAuthorizedOrNotFound" on a
// cluster OCID), so this creates/reuses a bastion session against the
// "jump" Compute instance sitting in the bastion's own VCN
// (findJumpInstance) instead, then chains one more plain ssh hop from that
// jump instance to the DB node's own host IP (buildChainedSSHCommand) —
// exactly the two-hop path this tenancy's own ~/.ssh/config already proves
// out by hand (bastion -> jump -> node). skipReuse mirrors createSession's.
func (m Model) createExascaleNodeSession(bastionID string, row registry.Row, node exascaleNodeRow, username string, key sshKeyPair, skipReuse bool) tea.Cmd {
	factory := m.factory
	scope := m.scope
	return func() tea.Msg {
		ctx := context.Background()

		if node.ip == "" {
			return sessionReadyMsg{err: fmt.Errorf("db node has no resolved host IP")}
		}

		jump, err := findJumpInstance(ctx, factory, scope, bastionID)
		if err != nil {
			return sessionReadyMsg{err: fmt.Errorf("find jump host: %w", err)}
		}
		jumpIP, err := instancePrivateIP(ctx, factory, scope, jump.id)
		if err != nil {
			return sessionReadyMsg{err: fmt.Errorf("resolve jump host private IP: %w", err)}
		}

		// Cache key stays row.ID-based (the DB node's own identity), same
		// as the local fast-path check in updatePrompt — the bastion
		// session it maps to is scoped to the shared jump host, so several
		// different nodes' cache entries can legitimately point at (and
		// reuse) that one same session.
		cacheKey := bastionSessionCacheKey(bastionID, row.ID, username)

		var session oci_bastion.Session
		var expiresAt time.Time
		reused := false
		if !skipReuse {
			if existing, exp, ok := findReusableSession(ctx, factory, scope, bastionID, jump.id, username); ok {
				session, expiresAt, reused = existing, exp, true
			}
		}
		if !reused {
			createdAt := time.Now()
			session, err = createBastionSession(ctx, factory, scope, bastionID, jump.id, jumpIP, username, key.pubKeyContent)
			if err != nil {
				return sessionReadyMsg{err: fmt.Errorf("create bastion session: %w", err)}
			}
			expiresAt = createdAt.Add(bastionSessionTTLSeconds * time.Second)
		}

		sshCmd, err := buildChainedSSHCommand(session, scope.Region, jumpIP, node.ip, username, key.privateKeyPath)
		if err != nil {
			return sessionReadyMsg{err: err}
		}
		return sessionReadyMsg{sshCmd: sshCmd, cacheKey: cacheKey, expiresAt: expiresAt, reused: reused}
	}
}

// createDirectSession skips the OCI Bastion service entirely and ssh's
// straight to the target's private IP with the already-resolved local
// key — useful when the caller's network already reaches the VCN directly
// (e.g. on-prem over FastConnect) and a bastion hop isn't needed. There's no
// server-side session resource here, so nothing to cache.
func (m Model) createDirectSession(row registry.Row, username string, key sshKeyPair) tea.Cmd {
	factory := m.factory
	scope := m.scope
	return func() tea.Msg {
		ctx := context.Background()

		privateIP, err := targetPrivateIP(ctx, factory, scope, row)
		if err != nil {
			return sessionReadyMsg{err: fmt.Errorf("resolve target private IP: %w", err)}
		}
		return sessionReadyMsg{sshCmd: buildDirectSSHCommand(username, privateIP, key.privateKeyPath)}
	}
}

// resolveSSHKey runs right after the ssh mode (direct/bastion) is picked:
// with exactly one local key candidate there's nothing to ask, so it's
// auto-selected and setup continues immediately; with more than one, a
// picker asks which one connects to this target before continuing.
func (m *Model) resolveSSHKey() (tea.Model, tea.Cmd) {
	keys, err := listSSHKeyPairs()
	if err != nil {
		m.statusMsg = err.Error()
		return *m, nil
	}
	if len(keys) == 1 {
		m.sshKey = keys[0]
		return *m, m.continueSSHSetup()
	}
	m.sshKeyPairs = keys
	items := make([]pickerItem, len(keys))
	for i, k := range keys {
		items[i] = pickerItem{key: k.privateKeyPath, label: k.name}
	}
	m.picker = newPicker(pickerSSHKey, "ssh key", items)
	m.mode = modePicker
	return *m, nil
}

// continueSSHSetup runs once both the ssh mode and the local key to use are
// resolved: a direct session just needs the OS username next, a bastion
// session needs a bastion picked (or, with a cached session already live
// for it, nothing further at all — see updatePrompt).
func (m *Model) continueSSHSetup() tea.Cmd {
	if m.sshDirect {
		m.promptInput.SetValue("opc")
		m.promptInput.CursorEnd()
		m.promptInput.Focus()
		m.mode = modePrompt
		return nil
	}
	m.statusMsg = "looking for a bastion..."
	return m.fetchBastions()
}

// embTermSize returns the cols/rows the embedded terminal should use to
// exactly fill the space View() renders it into: mainContentWidth wide,
// and the same vertical budget as the detail viewport minus one line for
// the hint text rendered below it.
func (m Model) embTermSize() (cols, rows int) {
	// Rendered inside a bordered box (see renderEmbTermBox), same layout
	// shape as the resource table box: tableBoxOverhead off the width
	// (border+padding), and the same -10 off the height as m.tableHeight —
	// box border (2) + a blank line + the hint line below it is exactly
	// the same 4-line overhead the table box's own border+blank+status
	// line accounts for.
	cols = m.mainContentWidth() - tableBoxOverhead
	rows = m.height - 10
	if cols < 10 {
		cols = 10
	}
	if rows < 5 {
		rows = 5
	}
	return cols, rows
}

// renderEmbTermBox wraps the embedded terminal's rendered content in the
// same rounded-border box style as the resource table, with a centered
// title — so an ssh session reads as its own window rather than the
// table's frame just vanishing.
func (m Model) renderEmbTermBox(content string) string {
	// vt.Emulator.Render() trims each line at its last non-empty cell
	// rather than padding to the full column count (fine for reading the
	// text back out, but without an explicit Width here lipgloss shrinks
	// the box to fit whatever's actually been printed so far — a mostly
	// blank prompt collapses the whole box to a sliver).
	cols, _ := m.embTermSize()
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ociBorder)).
		Padding(0, 1).
		Width(cols)
	lines := strings.Split(style.Render(content), "\n")

	title := titleStyle.Render(" SSH: " + m.pendingRow.Name + " ")
	topWidth := ansi.StringWidth(lines[0])
	x := (topWidth - ansi.StringWidth(title)) / 2
	if x < 0 {
		x = 0
	}
	lines[0] = embedInLine(lines[0], title, x)
	return strings.Join(lines, "\n")
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
	s.Header = s.Header.Bold(true).Foreground(lipgloss.Color(ociHighlt))
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
// even when actual values are far shorter (e.g. PUBLIC IP declares room
// for a full IPv4 address, but plenty of instances have none), which both
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

// protectedColumnHeaders are the column titles colorizeState/colorizeEdition
// (state_color.go/edition_color.go) substring-match against the fully
// rendered cell text, after the table itself has already truncated it.
// Shrinking one of these below its content-fit width risks bubbles cutting
// a value like "Running" down to "Runni…" — the match then silently fails
// and the color just vanishes, with nothing visibly wrong to explain why.
// fitColumns exempts them from shrinking; every other column absorbs the
// difference instead.
var protectedColumnHeaders = map[string]bool{
	"STATE":   true,
	"NODE":    true,
	"EDITION": true,
}

// fitColumns computes each column's content-fit width (fitColumnWidth), then
// scales every column proportionally against the available viewport width —
// down if it's too wide, rather than leaving columns at full size and
// letting bubbles silently crop whatever falls past the right edge; up if
// there's slack, rather than leaving it as dead space (which is what used
// to make the selected-row highlight stop short of the table box's right
// edge, and left values that would otherwise fit the screen capped at "…"
// for no reason). Scaling every column by the same ratio, instead of
// dumping all the slack or all the shrinkage into one column, keeps the
// table's proportions the same as the terminal grows or shrinks — except
// when shrinking, where protectedColumnHeaders opt out (see shrinkColumns).
func fitColumns(cols []registry.Column, colValues [][]string, available int) []int {
	natural := make([]int, len(cols))
	total := 0
	for i, c := range cols {
		natural[i] = fitColumnWidth(c.Header, colValues[i], c.Width)
		total += natural[i] + 2
	}
	if available <= 0 || total == available || len(cols) == 0 {
		return natural
	}
	if total > available {
		return shrinkColumns(cols, natural, available)
	}

	padding := 2 * len(cols)
	contentAvailable := available - padding
	if contentAvailable < tableColMinWidth*len(cols) {
		contentAvailable = tableColMinWidth * len(cols)
	}
	contentTotal := total - padding
	if contentTotal <= 0 {
		return natural
	}
	scale := float64(contentAvailable) / float64(contentTotal)

	widths := make([]int, len(cols))
	sum := 0
	for i, w := range natural {
		nw := int(float64(w) * scale)
		if nw < tableColMinWidth {
			nw = tableColMinWidth
		}
		widths[i] = nw
		sum += nw
	}
	// Integer truncation during the scale-up can leave a few columns short
	// of contentAvailable — hand the small remainder to the last column so
	// the row still reaches the box's right edge exactly.
	widths[len(widths)-1] += contentAvailable - sum
	return widths
}

// shrinkColumns is fitColumns' too-wide case: every protectedColumnHeaders
// column keeps its full natural width, and the shortfall is scaled
// proportionally across the rest — the same distribute-by-ratio idea
// fitColumns itself uses, just applied to a subset.
func shrinkColumns(cols []registry.Column, natural []int, available int) []int {
	widths := make([]int, len(cols))
	protectedTotal, flexTotal, flexCount := 0, 0, 0
	for i, c := range cols {
		if protectedColumnHeaders[c.Header] {
			widths[i] = natural[i]
			protectedTotal += natural[i] + 2
		} else {
			flexTotal += natural[i] + 2
			flexCount++
		}
	}

	flexAvailable := available - protectedTotal
	if flexCount == 0 || flexAvailable <= tableColMinWidth*flexCount {
		// Nothing flexible to negotiate with, or the protected columns
		// alone already fill (or exceed) the terminal — floor every
		// flexible column and let bubbles clip the rest, same as an
		// unworkably narrow terminal already does elsewhere.
		for i, c := range cols {
			if !protectedColumnHeaders[c.Header] {
				widths[i] = tableColMinWidth
			}
		}
		return widths
	}

	padding := 2 * flexCount
	contentAvailable := flexAvailable - padding
	if contentAvailable < tableColMinWidth*flexCount {
		contentAvailable = tableColMinWidth * flexCount
	}
	contentTotal := flexTotal - padding
	scale := float64(contentAvailable) / float64(contentTotal)

	sum, lastFlex := 0, -1
	for i, c := range cols {
		if protectedColumnHeaders[c.Header] {
			continue
		}
		nw := int(float64(natural[i]) * scale)
		if nw < tableColMinWidth {
			nw = tableColMinWidth
		}
		widths[i] = nw
		sum += nw
		lastFlex = i
	}
	if lastFlex >= 0 {
		widths[lastFlex] += contentAvailable - sum
	}
	return widths
}

// refreshTable rebuilds m.table from the resource's current columns and the
// given rows, preserving the table's on-screen size.
func (m *Model) refreshTable(rows []registry.Row) {
	cols := m.displayColumns()
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
	var rows []registry.Row
	if m.subtreeOn {
		// Placeholder ("— loading —") rows go through applyFilter's fuzzy
		// match on Name like anything else, so they'd otherwise wink in
		// and out of a filtered view depending on how the placeholder text
		// happens to score — keep them out of the filter and always shown.
		var placeholders []registry.Row
		var real []registry.Row
		for _, r := range m.buildSubtreeRows() {
			if isSubtreePlaceholderRow(r) {
				placeholders = append(placeholders, r)
			} else {
				real = append(real, r)
			}
		}
		rows = append(applyFilter(real, m.filterQuery), placeholders...)
	} else {
		rows = applyFilter(m.rows, m.filterQuery)
	}
	if m.groupingActive() {
		rows = groupRowsByVcn(rows, m.vcnNames)
	}
	if m.exascaleNodeTreeActive() {
		rows = expandExascaleNodes(rows)
	}
	m.displayRows = rows
	m.refreshTable(m.displayRows)
}

// mainAbsFloor is the main panel's true minimum width.
const mainAbsFloor = 10

// tableBoxOverhead is the two border columns plus the two 1-space padding
// columns (left + right of each) renderTableBox adds around the table in
// View() — reserved off the table's own width so the boxed table doesn't
// overflow mainContentWidth. The padding matters here specifically: without
// it the STATE column's colored text (colorizeState paints its whole cell,
// padding included) sat flush against the right border while
// the left side still had bubbles' own column padding as a visible gap —
// this box-level padding gives both sides the same margin regardless of
// what's colored inside.
const tableBoxOverhead = 4

// mainContentWidth is how many columns the main panel (table/detail/status
// line) has to work with — the terminal width minus a blank margin on each
// side (see View()) so the table box isn't flush against either edge.
func (m Model) mainContentWidth() int {
	w := m.width - 4
	if w < mainAbsFloor {
		w = mainAbsFloor
	}
	return w
}

// relayout recomputes the table/detail width against the terminal's current
// on-screen width. Needed on every window resize.
func (m *Model) relayout() {
	mainWidth := m.mainContentWidth()
	tableWidth := mainWidth - tableBoxOverhead
	if tableWidth < mainAbsFloor {
		tableWidth = mainAbsFloor
	}
	m.table.SetWidth(tableWidth)
	m.detail.SetWidth(mainWidth)
	m.relayoutTableColumns()
}

// relayoutTableColumns recomputes the table's column widths against its
// (already updated) width, in place. Unlike refreshTable, this calls
// table.Model.SetColumns directly instead of rebuilding via newTable, so it
// doesn't reset the cursor/scroll position — a pure width change (resizing
// the terminal) shouldn't jump the selection back to the top row the way a
// real row-set change (filter, reload) should.
func (m *Model) relayoutTableColumns() {
	cols := m.displayColumns()
	colValues := make([][]string, len(cols))
	for _, row := range m.displayRows {
		for j, c := range cols {
			colValues[j] = append(colValues[j], c.Get(row))
		}
	}
	widths := fitColumns(cols, colValues, m.table.Width())
	tcols := make([]table.Column, len(cols))
	for i, c := range cols {
		tcols[i] = table.Column{Title: c.Header, Width: widths[i]}
	}
	m.table.SetColumns(tcols)
}

// vcnScopedResourceKeys are the resource kinds that live "inside" a VCN —
// either natively filterable by VcnId, or joined against one client-side
// (see registry.Scope.VcnID and instance_vcn_filter.go). Picking a VCN row
// ("i" or Enter) scopes every resource in this set until a
// non-VCN-scoped one is picked; isVcnDependent is the single source of
// truth other code checks against.
var vcnScopedResourceKeys = map[string]bool{
	"vcn": true, "subnet": true, "route-table": true, "security-list": true,
	"nsg": true, "instance": true, "lb": true, "db-system": true, "adb": true, "exadata": true, "exascale": true,
}

// isVcnDependent reports whether switching to this resource should keep an
// active VCN filter (scope.VcnID) instead of clearing it.
func isVcnDependent(key string) bool {
	return vcnScopedResourceKeys[key]
}

// drgScopedResourceKeys is the DRG analog of vcnScopedResourceKeys — just
// DrgAttachments, since a DRG attachment isn't itself "inside" any other
// VCN-scoped resource the way Subnet/Instance/etc. are inside a VCN.
var drgScopedResourceKeys = map[string]bool{
	"drg": true, "drg-attachment": true, "drg-route-table": true, "drg-route-distribution": true,
}

// isDrgDependent reports whether switching to this resource should keep an
// active DRG filter (scope.DrgID) instead of clearing it.
func isDrgDependent(key string) bool {
	return drgScopedResourceKeys[key]
}

// groupingActive reports whether the "g" grouping column/sort should apply:
// only the Subnet view, and only with no VCN filter (a filtered list is
// already scoped to a single VCN, so grouping it would be a no-op).
func (m Model) groupingActive() bool {
	return m.groupByVcn && m.current().Key() == "subnet" && m.scope.VcnID == ""
}

// exascaleNodeTreeActive reports whether the "g" node tree should apply:
// only the Exascale view (see exascaleNodeTree's doc comment).
func (m Model) exascaleNodeTreeActive() bool {
	return m.exascaleNodeTree && m.current().Key() == "exascale"
}

// displayColumns is m.current().Columns(), tree-decorated (see vcn_tree.go)
// when groupingActive — the single source of truth for table shape, so
// refreshTable and relayoutTableColumns never disagree on column count
// against the row cells already sitting in m.table (that mismatch panics
// inside bubbles/table).
func (m *Model) displayColumns() []registry.Column {
	cols := m.current().Columns()
	switch {
	case m.groupingActive():
		cols = treeColumns(cols, treeGlyphs(m.displayRows))
	case m.exascaleNodeTreeActive():
		cols = exascaleTreeColumns(cols, exascaleTreeGlyphs(m.displayRows))
	}
	// Subtree mode's COMPARTMENT column goes in front of whatever the
	// above already built, last — it needs the final column set to wrap.
	if m.subtreeActive() {
		cols = subtreeColumns(cols)
	}
	return cols
}

// fetchVcnNames lists every VCN in the current compartment and returns a
// VcnID->vcnInfo map, for vcnLabel/groupRowsByVcn to use once loaded.
func (m Model) fetchVcnNames() tea.Cmd {
	factory := m.factory
	scope := m.scope
	scope.VcnID = ""
	return func() tea.Msg {
		rows, err := fetchAll(context.Background(), registry.NewVcnResource(factory), scope)
		if err != nil {
			return vcnNamesMsg{err: err}
		}
		infos := make(map[string]vcnInfo, len(rows))
		for _, row := range rows {
			cidr := ""
			if v, ok := row.Raw.(core.Vcn); ok {
				cidr = deref(v.CidrBlock)
			}
			infos[row.ID] = vcnInfo{Name: row.Name, Cidr: cidr}
		}
		return vcnNamesMsg{names: infos}
	}
}

// drgIDRequiredResourceKeys are the DRG-scoped resources whose underlying
// OCI API takes DrgId as a *mandatory* parameter (unlike the VCN-scoped
// resources, e.g. Subnet, which work fine unfiltered) — so unlike every
// other resource in the app, they simply cannot be listed without a DRG
// already picked. switchResource redirects to the DRG list instead of
// loading these with an empty DrgID (which would just 404).
var drgIDRequiredResourceKeys = map[string]bool{
	"drg-route-table": true, "drg-route-distribution": true,
}

func (m *Model) switchResource(idx int) tea.Cmd {
	if drgIDRequiredResourceKeys[m.resources[idx].Key()] && m.scope.DrgID == "" {
		m.statusMsg = "select a DRG first (\"i\" or Enter on a DRG row)"
		for i, r := range m.resources {
			if r.Key() == "drg" {
				idx = i
				break
			}
		}
	}
	m.resIdx = idx
	m.rows = nil
	m.filterQuery = ""
	m.loading = true
	m.err = nil
	// A VCN filter stays active while moving between VCN-scoped resources
	// (Subnets, Instances, ...), so hopping between them via "f" doesn't
	// need re-picking the VCN each time. Switching to anything else
	// (Compartments, DRGs, or back to the VCN list itself) drops it.
	if !isVcnDependent(m.resources[idx].Key()) {
		m.scope.VcnID = ""
		m.vcnFilterName = ""
	}
	// Same idea for the DRG filter (DrgAttachments) — an independent axis
	// from the VCN filter above, so it's checked and cleared separately.
	if !isDrgDependent(m.resources[idx].Key()) {
		m.scope.DrgID = ""
		m.drgFilterName = ""
	}
	// Subtree mode (F3) fans out across every compartment in scope for
	// whatever resource is selected — switching resource re-fans-out
	// instead of a plain single-compartment load. startSubtreeFanout
	// resets subtreeRows/subtreeTargets and calls setDisplayRows() itself;
	// calling setDisplayRows() here first would render the *old* resource's
	// leftover subtree rows through the *new* resource's columns (a type
	// assertion mismatch that panics — see registry.Column.Get).
	if m.subtreeOn {
		return m.startSubtreeFanout()
	}
	m.setDisplayRows()
	return m.load()
}

// selectVcnFilter scopes every VCN-dependent resource (see
// isVcnDependent) to one VCN — triggered by "i" on a VCN row, or by Enter —
// then opens the resource search so the user can jump straight to one of
// them. It doesn't switch resource or reload itself: nothing needs
// fetching until a specific resource is picked.
func (m *Model) selectVcnFilter(id, name string) {
	m.scope.VcnID = id
	m.vcnFilterName = name
	m.openResourceSearch()
}

// selectDrgFilter is selectVcnFilter's DRG analog — triggered by "i" or
// Enter on a DRG row, scopes DrgAttachments to this DRG.
func (m *Model) selectDrgFilter(id, name string) {
	m.scope.DrgID = id
	m.drgFilterName = name
	m.openResourceSearch()
}

// openResourceSearch opens the "f"/":" centered fuzzy picker over every
// resource kind, grouped into categories (resourcePickerItems) the same
// way OCI's own console groups its left nav.
func (m *Model) openResourceSearch() {
	resources := m.resources
	currentKey := m.current().Key()
	p := newPicker(pickerResource, "Resources", nil)
	p.treeFilter = func(query string) []pickerItem {
		return resourcePickerItems(resources, currentKey, query)
	}
	p.refilter()
	// Land on the current resource rather than item 0 — with categories
	// in the way, item 0 is now a header with nothing to select.
	for i, it := range p.filtered {
		if it.key == currentKey {
			p.cursor = i
			break
		}
	}
	m.picker = p
	m.mode = modePicker
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

// exitDrg is exitVcn's DRG analog — clears the DRG scope and returns to the
// DRG list.
func (m *Model) exitDrg() tea.Cmd {
	idx := -1
	for i, r := range m.resources {
		if r.Key() == "drg" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	m.scope.DrgID = ""
	m.drgFilterName = ""
	return m.switchResource(idx)
}

// isRecentRow reports whether row was created within recentRowWindow.
func (m Model) isRecentRow(row registry.Row) bool {
	return row.TimeCreated.After(time.Now().Add(-recentRowWindow))
}

// recentRowNames is the set of displayed row names eligible to blink, for
// blinkRecentRows' content-match against the rendered table (see its doc).
func (m Model) recentRowNames() map[string]bool {
	names := make(map[string]bool)
	for _, row := range m.displayRows {
		if m.isRecentRow(row) {
			names[row.Name] = true
		}
	}
	return names
}

func (m *Model) selected() (registry.Row, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.displayRows) {
		return registry.Row{}, false
	}
	row := m.displayRows[i]
	if isSubtreePlaceholderRow(row) {
		return registry.Row{}, false
	}
	return row, true
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
		msg, err := a.RunAction(context.Background(), scope, spec.Key, row.ID)
		return actionResultMsg{label: spec.Label + " " + row.Name, msg: msg, err: err}
	}
}

func renderDetail(row registry.Row) string {
	b, err := yaml.Marshal(row.Raw)
	if err != nil {
		return fmt.Sprintf("failed to render detail: %v", err)
	}
	return string(b)
}

// openDetailView switches to the plain YAML detail view for row — shared by
// the "d" key and, since F6, Enter on a Compartments row.
func (m *Model) openDetailView(row registry.Row) {
	m.openDetailContent(renderDetail(row))
}

// openDetailContent switches to modeDetail showing content verbatim — the
// resource map ("M") uses this directly (it's already a fully rendered
// string, not a row to YAML-marshal); openDetailView is just this plus
// the YAML rendering step.
func (m *Model) openDetailContent(content string) {
	m.mode = modeDetail
	m.detail.SetContent(content)
	m.detail.GotoTop()
	m.detailExport = nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// -10/-8: 5 header lines (Profile/Region/Resource/Compartment/
		// Recent) + 1 blank line, plus whatever else each pane reserves
		// below that.
		m.tableHeight = msg.Height - 10
		m.table.SetHeight(m.tableHeight)
		m.detail.SetHeight(msg.Height - 8)
		m.relayout()
		if m.embTerm != nil {
			cols, rows := m.embTermSize()
			m.embTerm.resize(cols, rows)
		}
		return m, nil

	case splashTickMsg:
		if m.mode != modeSplash {
			return m, nil
		}
		m.splashFrame++
		target := splashStages[splashStageIndex(m.splashFrame)]
		if !m.splashDataReady && target >= 100 {
			target = splashStages[len(splashStages)-2]
		}
		if target > m.splashProgress {
			m.splashProgress = target
			m.splashPhrase = splashPhrases[rand.Intn(len(splashPhrases))]
			m.splashSpinnerFrame++
		}
		if m.splashDataReady && m.splashProgress >= 100 {
			// Compartments (resIdx's default) is an info-only view now
			// (F6) rather than the old navigation entry point, so landing
			// on it first has nothing useful to do — open the resource
			// search immediately instead, prompting a real pick.
			m.openResourceSearch()
			return m, nil
		}
		return m, splashTickCmd()

	case blinkTickMsg:
		m.blinkOn = !m.blinkOn
		return m, blinkTickCmd()

	case bastionSpinnerTickMsg:
		if !m.bastionPending {
			return m, nil
		}
		m.bastionSpinnerFrame++
		return m, bastionSpinnerTickCmd()

	case rowsMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.rows = msg.rows
			m.setDisplayRows()
			if m.restoreCursorID != "" {
				for i, row := range m.displayRows {
					if row.ID == m.restoreCursorID {
						m.table.SetCursor(i)
						break
					}
				}
				m.restoreCursorID = ""
			}
		}
		return m, nil

	case compartmentTreeMsg:
		return m.handleCompartmentTreeMsg(msg)

	case subtreeRowsMsg:
		return m.handleSubtreeRowsMsg(msg)

	case rootNameMsg:
		m.tenancyName = msg.name
		if len(m.compPath) > 0 {
			m.compPath[0].Name = msg.name
		}
		if m.compTree != nil {
			if n := m.compTree.node(m.compTree.root.ID); n != nil {
				n.Name = msg.name
			}
		}
		m.relayout()
		// The splash screen's "real data" gate: with no initial m.load()
		// (see Init()), this — not a rows fetch — is the first real
		// round-trip to finish, so it's what unblocks leaving splash.
		if m.mode == modeSplash {
			m.splashDataReady = true
		}
		return m, nil

	case vcnNamesMsg:
		if msg.err != nil {
			m.statusMsg = "vcn names failed: " + msg.err.Error()
			return m, nil
		}
		m.vcnNames = msg.names
		m.setDisplayRows()
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
		if msg.msg != "" {
			// Multi-line results (e.g. a plugin-status list) don't fit the
			// one-line status bar, which clips to terminal width — show
			// them in the scrollable detail view instead.
			m.mode = modeDetail
			m.detail.SetContent(msg.label + "\n\n" + colorizePluginStatusText(msg.msg))
			m.detail.GotoTop()
			m.detailExport = nil
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

	case resourceMapMsg:
		if msg.err != nil {
			m.statusMsg = "resource map failed: " + msg.err.Error()
			return m, nil
		}
		m.statusMsg = ""
		m.resourceMap = &msg.data
		m.resourceMapSelected = 0
		if len(msg.data.subnets) == 0 {
			m.resourceMapSelected = -1
		}
		m.openDetailContent(renderResourceMap(msg.data, m.resourceMapSelected))
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
		m.bastionPending = false
		if msg.err != nil {
			m.statusMsg = "ssh setup failed: " + msg.err.Error()
			return m, nil
		}
		if msg.cacheKey != "" {
			m.bastionSessions[msg.cacheKey] = cachedBastionSession{sshCmd: msg.sshCmd, expiresAt: msg.expiresAt}
		}
		cols, rows := m.embTermSize()
		et, err := startEmbeddedTerm(msg.sshCmd, cols, rows)
		if err != nil {
			m.statusMsg = "ssh setup failed: " + err.Error()
			return m, nil
		}
		m.statusMsg = ""
		m.embTerm = et
		m.activeSessionCacheKey = msg.cacheKey
		m.activeSessionWasReused = msg.reused
		m.mode = modeEmbeddedTerm
		return m, et.waitForActivity()

	case embTermActivityMsg:
		if m.embTerm == nil {
			return m, nil
		}
		return m, m.embTerm.waitForActivity()

	case embTermExitMsg:
		// A ctrl+\ force-quit already set its own statusMsg and switched
		// back to the table — the SIGKILL that triggers this message
		// always surfaces as a "signal: killed" error from cmd.Wait(),
		// which isn't an actual failure here, so leave that message alone.
		killed := m.embTerm != nil && m.embTerm.killed
		if !killed && msg.err != nil {
			// The session might be the thing that's actually broken
			// (e.g. expired early server-side) rather than this one
			// connection attempt — evict it so the next try creates a
			// fresh one instead of replaying the same failure for up
			// to bastionSessionTTLSeconds.
			if m.activeSessionCacheKey != "" {
				delete(m.bastionSessions, m.activeSessionCacheKey)
			}
			// A *reused* session that dies within quickFailWindow almost
			// always means the key picked for this attempt isn't the one
			// that session was registered with (see findReusableSession /
			// the in-memory cache in updatePrompt) — the session itself
			// is fine, this key just isn't authorized on it. toci has no
			// UI to create/manage bastion sessions directly, so the only
			// sensible recovery is to stop reusing and make a fresh
			// session with the key actually selected — once, automatically,
			// rather than surfacing exit status 255 and making the user
			// re-trigger the whole ssh flow by hand.
			if m.activeSessionWasReused && m.embTerm != nil && m.embTerm.quickFail() {
				m.statusMsg = "reused session rejected this key — creating a new bastion session for " + m.pendingRow.Name + "..."
				m.embTerm = nil
				m.activeSessionCacheKey = ""
				m.activeSessionWasReused = false
				m.bastionPending = true
				m.bastionSpinnerFrame = 0
				m.mode = modeTable
				return m, tea.Batch(m.createSession(m.sshBastionID, m.pendingRow, m.sshUsername, m.sshKey, true), bastionSpinnerTickCmd())
			}
			m.statusMsg = "ssh exited with error: " + msg.err.Error()
		} else if !killed {
			m.statusMsg = "ssh session closed"
		}
		m.embTerm = nil
		m.activeSessionCacheKey = ""
		m.activeSessionWasReused = false
		m.mode = modeTable
		return m, nil

	case tea.KeyPressMsg:
		switch m.mode {
		case modeSplash:
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
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
		case modeEmbeddedTerm:
			return m.updateEmbeddedTerm(msg)
		default:
			return m.updateTable(msg)
		}

	case tea.MouseMsg:
		switch m.mode {
		case modeTable:
			return m.updateMouse(msg)
		case modePicker:
			return m.updatePickerMouse(msg)
		}
		return m, nil

	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// updateEmbeddedTerm forwards every keystroke straight into the pty —
// ctrl+\ is the one reserved escape hatch, force-quitting the session
// (killing the child process) and returning to the table. There's no
// detach/reattach: unlike tmux, a normal remote `exit` (embTermExitMsg)
// is the expected way back.
func (m Model) updateEmbeddedTerm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+\\" {
		if m.embTerm != nil {
			// Leave m.embTerm set (just marked killed) rather than nil —
			// the embTermExitMsg that follows shortly after needs to see
			// the flag to know its "signal: killed" error isn't a real
			// failure.
			m.embTerm.close()
		}
		m.mode = modeTable
		m.statusMsg = "ssh session force-quit"
		return m, nil
	}
	// shift+up/down scrolls the scrollback locally instead of reaching the
	// remote shell — reserved the same way ctrl+\ is.
	if m.embTerm != nil {
		switch msg.String() {
		case "shift+up":
			m.embTerm.scrollUp(3)
			return m, nil
		case "shift+down":
			m.embTerm.scrollDown(3)
			return m, nil
		}
	}
	if m.embTerm != nil {
		if b := keyMsgToBytes(msg); b != nil {
			m.embTerm.resetScroll() // typing means "back to live", like a real terminal
			_, _ = m.embTerm.pty.Write(b)
		}
	}
	return m, nil
}

func (m Model) updateDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "v":
		// "v" toggles: it opened this rules view (security-list/route-table
		// — see updateTable's "v" case), so pressing it again closes the
		// same way esc/q already do, rather than needing a different key
		// to back out of what "v" got you into.
		m.mode = modeTable
		m.resourceMap = nil
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.resourceMap != nil && m.resourceMapSelected > 0 {
			m.resourceMapSelected--
			m.detail.SetContent(renderResourceMap(*m.resourceMap, m.resourceMapSelected))
			return m, nil
		}
	case "down", "j":
		if m.resourceMap != nil && m.resourceMapSelected < len(m.resourceMap.subnets)-1 {
			m.resourceMapSelected++
			m.detail.SetContent(renderResourceMap(*m.resourceMap, m.resourceMapSelected))
			return m, nil
		}
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

func (m Model) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeTable
		return m, nil
	case "enter":
		return m.confirmPicker()
	case "tab":
		// Compartment-tree-only (F5): select + force subtree mode on. For
		// every other picker kind, fall through — tab isn't bound there.
		if m.picker.kind == pickerCompartment {
			item, ok := m.picker.selected()
			m.mode = modeTable
			if !ok {
				return m, nil
			}
			return m, m.switchCompartment(item.key, item.label, boolPtr(true))
		}
	case "p":
		// Compartment-tree-only (F5): pin/unpin the highlighted node
		// without closing the picker. Every other picker kind treats "p"
		// as a normal filter character (falls through below).
		if m.picker.kind == pickerCompartment {
			if item, ok := m.picker.selected(); ok {
				m.recentList = togglePin(m.recentList, item.key, item.label)
				saveRecent(m.profile, m.recentList)
				m.picker.refilter()
			}
			return m, nil
		}
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

// confirmPicker acts on the picker's currently-highlighted item — the
// shared "commit this choice" step behind both the enter key and a mouse
// click on an item (see updatePickerMouse).
func (m Model) confirmPicker() (tea.Model, tea.Cmd) {
	item, ok := m.picker.selected()
	if !ok {
		m.mode = modeTable
		return m, nil
	}
	if m.picker.kind == pickerResource && item.key == "" {
		// A category header row (see resourcePickerItems) — nothing to
		// switch to, so leave the picker open rather than closing it on a
		// no-op selection.
		return m, nil
	}
	m.mode = modeTable
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
	case pickerSSHMode:
		m.sshDirect = item.key == "direct"
		return m.resolveSSHKey()
	case pickerSSHKey:
		for _, k := range m.sshKeyPairs {
			if k.privateKeyPath == item.key {
				m.sshKey = k
				break
			}
		}
		return m, m.continueSSHSetup()
	case pickerResource:
		for i, res := range m.resources {
			if res.Key() == item.key {
				return m, m.switchResource(i)
			}
		}
	case pickerCompartment:
		return m, m.switchCompartment(item.key, item.label, nil)
	}
	return m, nil
}

// pickerRegularItemsTop/pickerResourceItemsTop are how many lines into the
// picker box (below its own top border) the first filtered item is drawn —
// see renderPicker (title, input, blank, then items) and
// renderResourceSearch (input, divider, then items).
const (
	pickerRegularItemsTop  = 3
	pickerResourceItemsTop = 2
)

// pickerItemAt maps a mouse click to a filtered-picker item index. box is
// the exact string the picker was rendered as (so the click can be bounded
// to its actual on-screen width/height); boxX/boxY is that box's top-left
// screen position; itemsTop is one of the constants above.
func pickerItemAt(clickX, clickY, boxX, boxY, itemsTop int, box string) (int, bool) {
	boxWidth, boxLines := overlayBoxDims(box)
	x, y := clickX-boxX, clickY-boxY
	if x < 0 || x >= boxWidth || y < 0 || y >= len(boxLines) {
		return 0, false
	}
	i := y - 1 - itemsTop // -1 for the box's own top border line
	if i < 0 {
		return 0, false
	}
	return i, true
}

// updatePickerMouse handles a left-click on a picker overlay (region/action/
// bastion/ssh/resource-search — see picker.go): it maps the click to a
// filtered item exactly the way each is actually drawn in View(), then
// commits it the same way pressing enter would.
func (m Model) updatePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return m, nil
	}
	// ponytail: click-to-select unsupported for the tree picker — its rows
	// don't map onto pickerItemAt's flat-list math (scrolling window,
	// variable indentation). Add if this picker turns out to need mouse
	// support in practice; keyboard nav covers it for now.
	if m.picker.kind == pickerCompartment {
		return m, nil
	}

	var box string
	var boxX, boxY, itemsTop int
	if m.picker.kind == pickerResource {
		// Overlaid centered over the table — see View()'s overlayCenter call.
		box = m.renderResourceSearch()
		boxWidth, boxLines := overlayBoxDims(box)
		boxX = (m.width - boxWidth) / 2
		boxY = (m.height - len(boxLines)) / 3
		itemsTop = pickerResourceItemsTop
	} else {
		// Drawn inline as the main panel, itself preceded by the header
		// block's 5 lines + 1 blank line and the "  " left margin — see
		// View()'s modePicker branch and mouseBodyTop's own comment.
		box = m.renderPicker()
		boxX, boxY = 2, 6
		itemsTop = pickerRegularItemsTop
	}

	i, ok := pickerItemAt(click.X, click.Y, boxX, boxY, itemsTop, box)
	if !ok || i >= len(m.picker.filtered) {
		return m, nil
	}
	m.picker.cursor = i
	if m.picker.kind == pickerResource {
		// The resource-search tree has category headers sitting between
		// real resources — a click there is easy to land near by accident,
		// and confirming immediately closed the whole picker on those. Just
		// move the cursor; Enter (or a second click landing on the same
		// row, since re-clicking the highlighted row is now indistinguishable
		// from clicking to select it) commits, same as keyboard nav.
		return m, nil
	}
	return m.confirmPicker()
}

func (m Model) updateFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m Model) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m Model) updatePrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		m.sshUsername = username
		if m.sshDirect {
			m.statusMsg = "connecting directly to " + m.pendingRow.Name + "..."
			return m, m.createDirectSession(m.pendingRow, username, m.sshKey)
		}
		cacheKey := bastionSessionCacheKey(m.sshBastionID, m.pendingRow.ID, username)
		if cached, ok := m.bastionSessions[cacheKey]; ok && time.Now().Add(bastionSessionReuseMargin).Before(cached.expiresAt) {
			m.statusMsg = "reusing existing bastion session for " + m.pendingRow.Name + "..."
			return m, func() tea.Msg {
				return sessionReadyMsg{sshCmd: cached.sshCmd, cacheKey: cacheKey, expiresAt: cached.expiresAt, reused: true}
			}
		}
		m.statusMsg = "connecting to bastion for " + m.pendingRow.Name + "..."
		m.bastionPending = true
		m.bastionSpinnerFrame = 0
		return m, tea.Batch(m.createSession(m.sshBastionID, m.pendingRow, username, m.sshKey, false), bastionSpinnerTickCmd())
	}
	var cmd tea.Cmd
	m.promptInput, cmd = m.promptInput.Update(msg)
	return m, cmd
}

// mouseBodyTop is the screen row where the table's first data row starts
// in modeTable's default render path: 5 header lines (Profile/Region/
// Resource/Compartment/Recent) + 1 blank line (see WindowSizeMsg's "-10"
// comment) + the table box's own top border + its column-header row.
const mouseBodyTop = 6 + 2

// updateMouse handles wheel scroll and left-click row selection over the
// resource table. bubbles' table.Model (v1.0.0) has no mouse support and
// keeps its scroll offset private, so a click is mapped to a row by
// diffing against the one line we can always find on screen — the
// cursor's own highlighted row, via selectedLinePrefix (already used by
// whitenDataRows/colorizeState) — instead of reimplementing its internal
// viewport math.
func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.table.MoveUp(1)
		case tea.MouseWheelDown:
			m.table.MoveDown(1)
		}

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || m.loading || m.err != nil || selectedLinePrefix == "" {
			return m, nil
		}
		lines := strings.Split(m.View().Content, "\n")
		selY := -1
		for i, line := range lines {
			if strings.Contains(line, selectedLinePrefix) {
				selY = i
				break
			}
		}
		if selY == -1 {
			return m, nil
		}
		if row, ok := mouseClickRow(msg.Y, selY, m.table.Cursor(), m.table.Height()); ok {
			m.table.SetCursor(row)
		}
	}
	return m, nil
}

// mouseClickRow maps a clicked screen row (clickY) to an absolute table
// row index, given the screen row the cursor is currently highlighted on
// (cursorY) and its absolute row index (cursorRow). Both are lines in the
// same contiguous, one-line-per-row table body, so the row delta equals
// the screen-line delta — no need to know the table's scroll offset. ok is
// false when the click falls outside the table body.
func mouseClickRow(clickY, cursorY, cursorRow, bodyHeight int) (row int, ok bool) {
	bodyIdx := clickY - mouseBodyTop
	if bodyIdx < 0 || bodyIdx >= bodyHeight {
		return 0, false
	}
	return cursorRow + (clickY - cursorY), true
}

// rowClipboardID returns the OCID "y" should copy for row, or ok=false for
// a row with nothing sensible to copy. row.ID is a synthetic
// "clusterID:nodeID" table key for an Exascale DB node child row (see
// exascale_tree.go), not a real OCID — copy the node's actual Id instead.
func rowClipboardID(row registry.Row) (id string, ok bool) {
	switch raw := row.Raw.(type) {
	case vcnGroupHeader:
		return "", false
	case exascaleNodeRow:
		return deref(raw.node.Id), true
	}
	return row.ID, true
}

// bigScrollRows is how many rows "shift+up"/"shift+down" jump per press —
// a LazyVim-style half-page scroll (like ctrl-d/ctrl-u) so paging through a
// long resource list doesn't mean holding down the arrow key. visibleRows
// is the table's own viewport height (m.table.Height()); at least 1 so a
// very short table still moves.
func bigScrollRows(visibleRows int) int {
	n := visibleRows / 2
	if n < 1 {
		n = 1
	}
	return n
}

func (m Model) updateTable(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	wasHelpOpen := m.showHelp

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
	case "shift+up":
		m.table.MoveUp(bigScrollRows(m.table.Height()))
		return m, nil

	case "shift+down":
		m.table.MoveDown(bigScrollRows(m.table.Height()))
		return m, nil

	case "space":
		// wasHelpOpen, not m.showHelp — the block above already closed it
		// unconditionally, so re-checking m.showHelp here would always see
		// false and reopen it every time, making space a no-op toggle.
		if !wasHelpOpen {
			m.showHelp = true
		}
		return m, nil

	case "ctrl+c", "q":
		return m, tea.Quit

	case ":", "f":
		m.openResourceSearch()
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

	case "b":
		m.blinkEnabled = !m.blinkEnabled
		if m.blinkEnabled {
			m.statusMsg = "blink: on"
		} else {
			m.statusMsg = "blink: off"
		}
		return m, nil

	case "g":
		switch m.current().Key() {
		case "subnet":
			if m.scope.VcnID != "" {
				return m, nil
			}
			m.groupByVcn = !m.groupByVcn
			var cmd tea.Cmd
			if m.groupByVcn {
				m.statusMsg = "group by vcn: on"
				if m.vcnNames == nil {
					cmd = m.fetchVcnNames()
				}
			} else {
				m.statusMsg = "group by vcn: off"
			}
			m.setDisplayRows()
			return m, cmd

		case "exascale":
			m.exascaleNodeTree = !m.exascaleNodeTree
			if m.exascaleNodeTree {
				m.statusMsg = "show nodes: on"
			} else {
				m.statusMsg = "show nodes: off"
			}
			m.setDisplayRows()
			return m, nil
		}
		return m, nil

	case "e":
		path := exportFilename(m.current().Key(), time.Now())
		rows := m.displayRows
		if m.groupingActive() {
			rows = filterOutGroupHeaders(rows)
		}
		if m.exascaleNodeTreeActive() {
			rows = filterOutExascaleNodes(rows)
		}
		if m.subtreeActive() {
			rows = filterOutSubtreePlaceholders(rows)
		}
		if err := exportCSV(path, m.current().Columns(), rows); err != nil {
			m.statusMsg = "export failed: " + err.Error()
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("exported %d rows to %s", len(rows), path)
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

	case "c":
		m.openCompartmentPicker()
		return m, nil

	case "C":
		m.subtreeOn = !m.subtreeOn
		if !m.subtreeOn {
			m.stopSubtreeFanout()
			m.statusMsg = "subtree: off"
			m.rows = nil
			m.loading = true
			return m, m.load()
		}
		if m.compTree == nil {
			m.subtreeOn = false
			m.statusMsg = "compartment tree still loading — try again shortly"
			return m, nil
		}
		m.statusMsg = "subtree: on"
		return m, m.startSubtreeFanout()

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(key[0] - '1')
		if idx >= len(m.recentList) {
			return m, nil
		}
		e := m.recentList[idx]
		return m, m.switchCompartment(e.ID, e.Name, boolPtr(e.Subtree))

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
		resKey := m.current().Key()
		if resKey != "instance" && resKey != "exascale" {
			return m, nil
		}
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		if resKey == "exascale" {
			if _, isNode := row.Raw.(exascaleNodeRow); !isNode {
				m.statusMsg = "select a DB node (press \"g\" to expand the node tree) to ssh"
				return m, nil
			}
		}
		m.pendingRow = row
		m.picker = newPicker(pickerSSHMode, "ssh: "+row.Name, []pickerItem{
			{key: "bastion", label: "via Bastion service"},
			{key: "direct", label: "direct (local key, no bastion — e.g. over FastConnect)"},
		})
		m.mode = modePicker
		return m, nil

	case "i":
		key := m.current().Key()
		if key != "vcn" && key != "drg" {
			return m, nil
		}
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		if key == "vcn" {
			m.selectVcnFilter(row.ID, row.Name)
		} else {
			m.selectDrgFilter(row.ID, row.Name)
		}
		return m, nil

	case "v":
		resKey := m.current().Key()
		if resKey != "security-list" && resKey != "route-table" {
			return m, nil
		}
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		var content string
		var records [][]string
		var headers []string
		var name, suffix string
		switch resKey {
		case "security-list":
			content, records, name, ok = securityRulesView(row)
			headers, suffix = securityRuleHeaders, "security-rules-"
		case "route-table":
			content, records, name, ok = routeRulesView(row)
			headers, suffix = routeRuleHeaders, "route-rules-"
		}
		if !ok {
			return m, nil
		}
		m.mode = modeDetail
		m.detail.SetContent(content)
		m.detail.GotoTop()
		m.detailExport = &detailExportData{
			filenameSuffix: suffix + name,
			header:         headers,
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

	case "M":
		if m.vcnFilterName == "" {
			m.statusMsg = "pick a VCN first (\"i\" on a VCN row) to view its resource map"
			return m, nil
		}
		m.statusMsg = "building resource map..."
		return m, m.buildResourceMap()

	case "d":
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		if _, isGroupHeader := row.Raw.(vcnGroupHeader); isGroupHeader {
			return m, nil
		}
		m.openDetailView(row)
		return m, nil

	case "y":
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		id, ok := rowClipboardID(row)
		if !ok {
			return m, nil
		}
		m.statusMsg = "copied " + id
		return m, tea.SetClipboard(id)

	case "enter":
		row, ok := m.selected()
		if !ok {
			return m, nil
		}
		switch m.current().Key() {
		// F6: Compartments is now an information-only view — Enter opens
		// detail instead of changing the active compartment. Every context
		// switch goes through "c" instead (F1), so there's exactly one way
		// to do it.
		case "compartment":
			m.openDetailView(row)
		case "vcn":
			m.selectVcnFilter(row.ID, row.Name)
		case "drg":
			m.selectDrgFilter(row.ID, row.Name)
		}
		return m, nil

	case "esc":
		if m.subtreeOn && m.subtreeLoadedCount() < len(m.subtreeTargets) {
			if m.subtreeCancel != nil {
				m.subtreeCancel()
			}
			m.statusMsg = "subtree fetch cancelled"
			return m, nil
		}
		if m.filterQuery != "" {
			m.filterQuery = ""
			m.setDisplayRows()
			return m, nil
		}
		if m.scope.VcnID != "" {
			return m, m.exitVcn()
		}
		if m.scope.DrgID != "" {
			return m, m.exitDrg()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// embTermContentRow/Col is the screen position of the embedded terminal's
// own row/col (0,0) — the header block (6 lines, incl. the blank
// separator) plus the terminal box's top border, and the outer "  " margin
// plus the box's border+padding. Same derivation as mouseBodyTop for the
// resource table; verified against a rendered frame rather than just
// counted by hand.
const (
	embTermContentRow = 7
	embTermContentCol = 4
)

// View builds the current screen and declares the terminal features this
// app needs (alt screen, cell-motion mouse tracking) — v1 set those via
// tea.NewProgram options; v2 moved them here as declarative View fields.
// It also places the real terminal cursor over the embedded SSH session's
// own cursor: v2's Cursor field is nil (hidden) unless set explicitly, so
// without this the ssh session looks like it has no cursor at all.
func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if m.mode == modeEmbeddedTerm && m.embTerm != nil && m.embTerm.scrollback == 0 {
		// ponytail: always a blinking block cursor — doesn't track the
		// remote app's own hide-cursor/shape requests (vt.Callbacks'
		// CursorVisibility/CursorStyle, unwired). Upgrade path: register
		// those callbacks in startEmbeddedTerm and store the result on
		// embeddedTerm if a full-screen remote app (vim, less) hiding its
		// cursor turns out to matter in practice.
		pos := m.embTerm.emu.CursorPosition()
		v.Cursor = tea.NewCursor(pos.X+embTermContentCol, pos.Y+embTermContentRow)
	}
	return v
}

func (m Model) viewContent() string {
	if m.mode == modeSplash {
		return renderSplash(m)
	}

	var b strings.Builder

	// "  " left margin matches the table box's own left margin below, so
	// the header block and the box line up on the same left edge instead
	// of the header sitting flush against the terminal border.
	b.WriteString(pathStyle.Render("  Profile:  "))
	b.WriteString(headerValueStyle.Render(m.profile))
	b.WriteString("\n")
	b.WriteString(pathStyle.Render("  Region:   "))
	b.WriteString(headerValueStyle.Render(m.scope.Region))
	b.WriteString("\n")
	b.WriteString(pathStyle.Render("  Resource: "))
	b.WriteString(headerValueStyle.Render(m.current().Label()))
	b.WriteString("\n")
	compartment := breadcrumbLabel(m.compPath)
	if m.vcnFilterName != "" {
		compartment += " › " + m.vcnFilterName
	}
	if m.drgFilterName != "" {
		compartment += " › " + m.drgFilterName
	}
	b.WriteString(pathStyle.Render("  Compartment: "))
	b.WriteString(headerValueStyle.Render(compartment))
	b.WriteString(m.subtreeBadge())
	b.WriteString("\n")
	b.WriteString(m.renderRecentLine())
	b.WriteString("\n\n")

	// The resource-search and compartment-tree pickers float centered over
	// the table instead of replacing it, so switching on m.mode alone
	// would wrongly blank the table out from under them — render as if
	// modeTable and overlay them after composing the full view below.
	renderMode := m.mode
	if renderMode == modePicker && (m.picker.kind == pickerResource || m.picker.kind == pickerCompartment) {
		renderMode = modeTable
	}

	var main strings.Builder
	switch renderMode {
	case modeDetail:
		main.WriteString(m.detail.View())
		main.WriteString("\n")
		hint := "esc: back"
		if m.resourceMap != nil {
			hint += " · j/k: select subnet"
		}
		if m.detailExport != nil {
			hint += " · e: export csv"
		}
		rendered := statusStyle.Render(hint)
		if m.statusMsg != "" {
			rendered += statusStyle.Render(" · ") + m.bastionSpinnerPrefix() + renderStatusMsg(m.statusMsg)
		}
		main.WriteString(rendered)

	case modePicker:
		main.WriteString(m.renderPicker())

	case modeConfirm:
		main.WriteString(m.renderConfirm())

	case modePrompt:
		main.WriteString(m.renderPrompt())

	case modeEmbeddedTerm:
		if m.embTerm != nil {
			_, rows := m.embTermSize()
			main.WriteString(m.renderEmbTermBox(m.embTerm.render(rows)))
			main.WriteString("\n")
		}
		hint := "ctrl+\\: force-quit · shift+↑/↓: scroll"
		if m.embTerm != nil && m.embTerm.scrollback > 0 {
			hint += fmt.Sprintf(" · scrolled back %d lines (shift+↓ to return)", m.embTerm.scrollback)
		}
		main.WriteString(statusStyle.Render(hint))

	default:
		switch {
		case m.err != nil:
			main.WriteString(errorStyle.Render("error: " + m.err.Error()))
		case m.loading:
			main.WriteString(statusStyle.Render("loading..."))
		default:
			tableView := whitenDataRows(m.table.View())
			tableView = colorizeState(tableView, m.table.Columns(), "STATE")
			tableView = colorizeState(tableView, m.table.Columns(), "NODE")
			tableView = colorizeEdition(tableView, m.table.Columns())
			if m.blinkEnabled {
				tableView = blinkRecentRows(tableView, m.table.Columns(), m.recentRowNames(), m.blinkOn)
			}
			main.WriteString(m.renderTableBox(tableView))
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

	// Blank margins on both sides so the table box isn't flush against
	// either edge of the terminal.
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, "  ", main.String(), "  "))

	out := b.String()
	// -2: right margin so the logo isn't flush against the terminal edge,
	// matching the table box's own right margin below.
	out = overlayTopRight(out, cornerLogo, m.width-2)
	out = overlayRightAt(out, headerValueStyle.Render(m.cornerSubtitle()), m.width-2, cornerLogoRows+2)
	if m.mode == modePicker && m.picker.kind == pickerResource {
		out = overlayCenter(out, m.renderResourceSearch(), m.width, m.height)
	}
	if m.mode == modePicker && m.picker.kind == pickerCompartment {
		out = overlayCenter(out, m.renderCompartmentPicker(), m.width, m.height)
	}
	if m.showHelp {
		out = overlayBottomRight(out, renderHelpBox(m), m.width)
	}
	return out
}

// cornerLogoArt is a compact 3-line block-font "TOCI", a scaled-down
// cousin of splash.go's big ANSI Shadow banner — same idea (a pixel-block
// wordmark), sized to sit in a corner instead of filling the splash screen.
const cornerLogoArt = `▄▄▄ ▄▄  ▄▄▄ ▄
 █  █ █ █   █
 █  ▀▄▀ ▀▄▄ ▄`

// cornerLogo is cornerLogoArt pinned to the top-right corner of the main
// screen (splash has its own big banner, so this only shows outside
// modeSplash — see View()'s early return there).
var cornerLogo = splashLogoStyle.Render(cornerLogoArt)

// cornerLogoRows is cornerLogoArt's own height — cornerSubtitle lands two
// rows past it (cornerLogoRows+2: one row of breathing room rather than
// sitting flush under the wordmark, plus one more for the "Recent:" line
// F2 added to the header block), landing on the header's blank separator
// line so it never fights the "Recent" line right above it for space.
// Derived from the art itself so resizing it doesn't leave the subtitle
// floating in the wrong place.
var cornerLogoRows = strings.Count(cornerLogoArt, "\n") + 1

// cornerSubtitle is the line under the corner wordmark: the release
// version for a real build (ldflags -X main.version=...), or "OCI TUI" for
// a "dev" (unversioned/local) build, where a version string would just be
// noise.
func (m Model) cornerSubtitle() string {
	if m.version == "" || m.version == "dev" {
		return "OCI TUI"
	}
	return m.version
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

// renderResourceSearch draws the "f" resource-search picker as a wide,
// Telescope-style box: a fixed width (not sized to content, unlike
// renderPicker) with the title and match count punched into the top border
// and a divider between the input and the results.
func (m Model) renderResourceSearch() string {
	width := m.width * 3 / 5
	if width < 50 {
		width = 50
	}
	if max := m.width - 4; width > max {
		width = max
	}
	// Terminal narrower than the box's floor (or not yet sized, e.g. before
	// the first WindowSizeMsg) — clamp instead of a negative Repeat count.
	if width < 20 {
		width = 20
	}

	var b strings.Builder
	b.WriteString("> ")
	b.WriteString(m.picker.input.View())
	b.WriteString("\n")
	// width-4, not width-2: width is the box's outer width, and the
	// divider sits inside both the border (1 col each side) and the
	// style's own padding (1 col each side) — see tableBoxOverhead for
	// the same accounting elsewhere. Off by those 2 extra columns, this
	// line was wide enough to make lipgloss soft-wrap it onto a second
	// row instead of just filling the divider's own row.
	b.WriteString(strings.Repeat("─", width-4))
	b.WriteString("\n")
	for i, it := range m.picker.filtered {
		// A category header (resourcePickerItems) has no key — nothing to
		// select, so it's dimmed via titleStyle instead of plain text, but
		// only when it isn't the highlighted row (selStyle's own styling
		// wins there, same as any other row — see embedTwoInLine's doc for
		// why nesting two Render calls' ANSI spans is worth avoiding).
		isCategory := it.key == ""
		text := it.glyph + it.label
		if !isCategory && it.isCurrent {
			text += " ●"
		}
		var line string
		switch {
		case i == m.picker.cursor:
			line = selStyle.Render("› " + text)
		case isCategory:
			line = "  " + titleStyle.Render(text)
		default:
			line = "  " + text
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(m.picker.filtered) == 0 {
		b.WriteString(statusStyle.Render("  (no matches)"))
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ociBorder)).
		Width(width).
		Padding(0, 1)
	lines := strings.Split(style.Render(strings.TrimRight(b.String(), "\n")), "\n")

	title := titleStyle.Render(" " + m.picker.title + " ")
	topWidth := ansi.StringWidth(lines[0])
	titleX := (topWidth - ansi.StringWidth(title)) / 2

	count := statusStyle.Render(fmt.Sprintf(" %d/%d ", pickerLeafCount(m.picker.filtered), len(m.resources)))
	countX := topWidth - ansi.StringWidth(count) - 1
	if countX > titleX+ansi.StringWidth(title) {
		lines[0] = embedTwoInLine(lines[0], title, titleX, count, countX)
	} else {
		lines[0] = embedInLine(lines[0], title, titleX)
	}

	return strings.Join(lines, "\n")
}

// renderTableBox draws the k9s-style bounding box around the resource
// table: an accent-colored rounded border with the resource label and row
// count punched into the center of its top edge. relayout reserves
// tableBoxOverhead columns off the table's own width for this border, so it
// never has to grow tableView to fit — it just wraps whatever's there.
func (m Model) renderTableBox(tableView string) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ociBorder)).
		Padding(0, 1)
	lines := strings.Split(style.Render(tableView), "\n")

	title := titleStyle.Render(fmt.Sprintf(" %s [%d] ", m.current().Label(), len(m.displayRows)))
	topWidth := ansi.StringWidth(lines[0])
	x := (topWidth - ansi.StringWidth(title)) / 2
	if x < 0 {
		x = 0
	}

	// F3: "↻ n/total loaded" while a subtree fan-out is still in flight —
	// disappears once every target compartment has reported back.
	if m.subtreeOn {
		if loaded, total := m.subtreeLoadedCount(), len(m.subtreeTargets); loaded < total {
			badge := statusStyle.Render(fmt.Sprintf(" ↻ %d/%d loaded ", loaded, total))
			badgeX := topWidth - ansi.StringWidth(badge) - 1
			if badgeX > x+ansi.StringWidth(title) {
				lines[0] = embedTwoInLine(lines[0], title, x, badge, badgeX)
				return strings.Join(lines, "\n")
			}
		}
	}
	lines[0] = embedInLine(lines[0], title, x)
	return strings.Join(lines, "\n")
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
	title := "SSH via Bastion: " + m.pendingRow.Name
	if m.sshDirect {
		title = "SSH direct: " + m.pendingRow.Name
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
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
	add("shift+↑/↓", "scroll half page")
	add("d", "detail")
	add("y", "copy OCID")
	add("f / :", "search resources")
	add("c", "compartment")
	if m.subtreeOn {
		add("C", "subtree: on")
	} else {
		add("C", "subtree: off")
	}
	add("/", "filter")
	add("r", "region")
	add("R", "refresh")
	if m.blinkEnabled {
		add("b", "blink: on")
	} else {
		add("b", "blink: off")
	}
	if m.current().Key() == "subnet" && m.scope.VcnID == "" {
		if m.groupByVcn {
			add("g", "group by vcn: on")
		} else {
			add("g", "group by vcn: off")
		}
	}
	if m.current().Key() == "exascale" {
		if m.exascaleNodeTree {
			add("g", "show nodes: on")
		} else {
			add("g", "show nodes: off")
		}
	}
	add("e", "export csv")
	if m.vcnFilterName != "" {
		add("m", "export diagram")
		add("M", "resource map")
	}
	if _, ok := m.actionable(); ok {
		if m.writeEnabled {
			add("a", "actions")
		} else {
			add("a", "actions (readonly)")
		}
	}
	if m.current().Key() == "instance" || (m.current().Key() == "exascale" && m.exascaleNodeTreeActive()) {
		if m.writeEnabled {
			add("s", "ssh")
		} else {
			add("s", "ssh (readonly)")
		}
	}
	if m.current().Key() == "vcn" {
		add("enter / i", "filter by this VCN")
	}
	if m.current().Key() == "drg" {
		add("enter / i", "filter by this DRG")
	}
	if key := m.current().Key(); key == "security-list" || key == "route-table" {
		add("v", "view rules")
	}
	if m.vcnFilterName != "" || m.drgFilterName != "" {
		add("esc", "up")
	} else if m.subtreeOn && m.subtreeLoadedCount() < len(m.subtreeTargets) {
		add("esc", "cancel subtree fetch")
	}
	add("q", "quit")
	return entries
}

// bastionSpinnerPrefix renders the animated spinner glyph (see splash.go's
// spinnerFrames/spinnerStyle) while a bastion session lookup/creation is in
// flight, so the status bar doesn't just sit on a static "connecting..."
// string for however long that OCI round-trip takes (up to ~90s worst
// case). Empty otherwise.
func (m Model) bastionSpinnerPrefix() string {
	if !m.bastionPending {
		return ""
	}
	return spinnerStyle.Render(spinnerFrames[m.bastionSpinnerFrame%len(spinnerFrames)]) + " "
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
// gray line spanning the terminal on every screen. "c: compartment" and
// "C: subtree ..." are the two exceptions (doc F1-F3): compartment context
// switching is common enough, and the subtree on/off state needs to be
// visible without opening the popup, that they earn a permanent spot here.
func (m Model) renderStatusLine() string {
	parts := []string{fmt.Sprintf("%d items", len(m.displayRows))}
	if m.filterQuery != "" {
		parts = append(parts, fmt.Sprintf("filter: %q", m.filterQuery))
	}
	parts = append(parts, "space: shortcuts", "c: compartment")
	if m.subtreeOn {
		parts = append(parts, "C: subtree on")
	} else {
		parts = append(parts, "C: subtree off")
	}
	line := statusStyle.Render(strings.Join(parts, " · "))
	if m.statusMsg != "" {
		line += statusStyle.Render(" · ") + m.bastionSpinnerPrefix() + renderStatusMsg(m.statusMsg)
	}
	return line
}

// subtreeBadge renders the "⊕ +N sub[, M skipped]" suffix for the
// Compartment header line while subtree mode (F3) is on — the same green
// as the footer's "C: subtree on" (doc: "모드는 두 곳에서 알린다").
func (m Model) subtreeBadge() string {
	if !m.subtreeOn {
		return ""
	}
	n := 0
	if base := m.compTree.node(m.scope.CompartmentID); base != nil {
		n = base.subtreeCount()
	}
	badge := fmt.Sprintf(" ⊕ +%d sub", n)
	if m.subtreeSkipped > 0 {
		badge += fmt.Sprintf(", %d skipped", m.subtreeSkipped)
	}
	return successStyle.Render(badge)
}

// renderRecentLine draws the header's "Recent:" line (F2): up to
// recentMaxSlots MRU compartments, "1".."9" in the table-header yellow
// (helpKeyStyle, same color newTable's own header uses), names in the
// default header color, "⊕" suffixed for one last visited in subtree mode.
func (m Model) renderRecentLine() string {
	prefix := pathStyle.Render("  Recent:     ")
	if len(m.recentList) == 0 {
		return prefix + statusStyle.Render(`(none yet — "c" to pick a compartment)`)
	}
	parts := make([]string, 0, len(m.recentList))
	for i, e := range m.recentList {
		if i >= recentMaxSlots {
			break
		}
		name := e.Name
		if e.Subtree {
			name += "⊕"
		}
		parts = append(parts, helpKeyStyle.Render(fmt.Sprintf("%d", i+1))+" "+headerValueStyle.Render(name))
	}
	return prefix + strings.Join(parts, "  ")
}
