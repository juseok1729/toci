package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/oracle/oci-go-sdk/v65/common"

	"toci/internal/registry"
)

// subtreePlaceholderPrefix marks a synthesized "— loading —" row's ID
// (subtreePlaceholderRow) — never a real OCID, so isSubtreePlaceholderRow
// is exact.
const subtreePlaceholderPrefix = "\x00subtree-placeholder:"

func isSubtreePlaceholderRow(row registry.Row) bool {
	return strings.HasPrefix(row.ID, subtreePlaceholderPrefix)
}

// subtreeConcurrency bounds how many compartments toci fetches from at
// once during a fan-out — OCI list APIs rate-limit per tenancy, and firing
// one goroutine per compartment unbounded would trip that on any tenancy
// with more than a handful of subcompartments.
const subtreeConcurrency = 6

// subtreeSem is package-level (not per-Model) so it throttles across the
// whole app, not per fan-out — matters if a stale fan-out's requests are
// still draining through the semaphore when a new one starts.
var subtreeSem = make(chan struct{}, subtreeConcurrency)

// subtreeTarget is one compartment a fan-out fetches rows from.
type subtreeTarget struct {
	ID       string
	RelLabel string // path relative to the fan-out's base compartment; "" for the base itself
}

// subtreeRowsMsg carries one target compartment's result back to Update.
// gen guards against a slow response from a fan-out the user has since
// replaced (new compartment, resource, or subtree toggle) — see
// startSubtreeFanout.
type subtreeRowsMsg struct {
	gen      int
	targetID string
	rows     []registry.Row
	err      error
}

// subtreeActive reports whether the subtree column/placeholder decoration
// should apply — the "C" toggle, independent of whether the fan-out it
// kicked off has finished.
func (m Model) subtreeActive() bool {
	return m.subtreeOn
}

// subtreeLoadedCount is how many of the current fan-out's targets have
// reported back (success, error, or cancellation) — the table title bar's
// "↻ n/total loaded" indicator.
func (m Model) subtreeLoadedCount() int {
	return len(m.subtreeDone)
}

// startSubtreeFanout cancels any fan-out already in flight and launches a
// fresh one over the current compartment plus every descendant the cached
// tree knows about, for whatever resource type is currently selected.
// Results stream back one subtreeRowsMsg per compartment rather than
// waiting for all of them, so the table shows real rows as they arrive
// (see F3's "부분 결과 우선" design principle).
func (m *Model) startSubtreeFanout() tea.Cmd {
	if m.subtreeCancel != nil {
		m.subtreeCancel()
	}
	m.subtreeGen++
	gen := m.subtreeGen
	ctx, cancel := context.WithCancel(context.Background())
	m.subtreeCancel = cancel

	base := m.compTree.node(m.scope.CompartmentID)
	targets := []subtreeTarget{{ID: m.scope.CompartmentID}}
	if base != nil {
		for _, d := range base.descendants() {
			targets = append(targets, subtreeTarget{ID: d.ID, RelLabel: m.compTree.relativePath(base.ID, d.ID)})
		}
	}

	m.subtreeTargets = targets
	m.subtreeRows = map[string][]registry.Row{}
	m.subtreeDone = map[string]bool{}
	m.subtreeSkipped = 0
	m.subtreeErrMsg = ""
	m.loading = false
	m.err = nil
	m.setDisplayRows()

	res := m.current()
	scope := m.scope
	cmds := make([]tea.Cmd, len(targets))
	for i, t := range targets {
		t := t
		cmds[i] = func() tea.Msg {
			subtreeSem <- struct{}{}
			defer func() { <-subtreeSem }()
			s := scope
			s.CompartmentID = t.ID
			rows, err := fetchAll(ctx, res, s)
			for i := range rows {
				rows[i].CompartmentLabel = t.RelLabel
			}
			return subtreeRowsMsg{gen: gen, targetID: t.ID, rows: rows, err: err}
		}
	}
	return tea.Batch(cmds...)
}

// stopSubtreeFanout cancels any in-flight fan-out and drops its state,
// used when "C" turns subtree mode back off or the app is quitting the
// view in a way that no longer needs it.
func (m *Model) stopSubtreeFanout() {
	if m.subtreeCancel != nil {
		m.subtreeCancel()
		m.subtreeCancel = nil
	}
	m.subtreeTargets = nil
	m.subtreeRows = nil
	m.subtreeDone = nil
	m.subtreeSkipped = 0
	m.subtreeErrMsg = ""
}

// handleSubtreeRowsMsg is Update's case for subtreeRowsMsg.
func (m Model) handleSubtreeRowsMsg(msg subtreeRowsMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.subtreeGen {
		return m, nil // stale — from a fan-out this one has since replaced
	}
	m.subtreeDone[msg.targetID] = true
	switch {
	case msg.err == nil:
		m.subtreeRows[msg.targetID] = msg.rows
	case errors.Is(msg.err, context.Canceled):
		// Esc-cancelled — drop silently, not a real error to report.
	default:
		if se, ok := common.IsServiceError(msg.err); ok && (se.GetHTTPStatusCode() == 401 || se.GetHTTPStatusCode() == 404) {
			m.subtreeSkipped++
		} else {
			m.subtreeErrMsg = summarizeSubtreeError(msg.err)
			m.statusMsg = "subtree fetch error (" + msg.targetID + "): " + m.subtreeErrMsg
		}
	}
	m.setDisplayRows()
	return m, nil
}

// summarizeSubtreeError condenses a fan-out error to one line for the
// status bar. A ServiceError's own Error() is the OCI Go SDK's full
// multi-paragraph troubleshooting block (status/code/message/operation
// name/timestamp/docs links, newlines included) — dumping that whole
// thing into what's meant to be a single status line broke the footer
// into several lines of raw SDK text instead of clipping to width like
// every other status message. Code + Message is the part a user actually
// needs; everything else is redundant with it.
func summarizeSubtreeError(err error) string {
	if se, ok := common.IsServiceError(err); ok {
		return fmt.Sprintf("%d %s: %s", se.GetHTTPStatusCode(), se.GetCode(), se.GetMessage())
	}
	// Not a ServiceError (context/network/etc.) — normally already a
	// short, single-line message, but guard against a multi-line one
	// anyway rather than trust that.
	msg := err.Error()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return msg
}

// buildSubtreeRows assembles the merged row set for subtree mode: each
// target compartment's own rows in fan-out order, or a single "— loading —"
// placeholder for a target that hasn't reported back yet.
func (m Model) buildSubtreeRows() []registry.Row {
	rows := make([]registry.Row, 0, len(m.subtreeTargets))
	for _, t := range m.subtreeTargets {
		if !m.subtreeDone[t.ID] {
			rows = append(rows, registry.Row{
				ID:               subtreePlaceholderPrefix + t.ID,
				Name:             "— loading —",
				CompartmentLabel: t.RelLabel,
			})
			continue
		}
		rows = append(rows, m.subtreeRows[t.ID]...)
	}
	return rows
}

// filterOutSubtreePlaceholders strips "— loading —" rows — used wherever
// rows are handed to code that assumes a real Row.Raw (CSV export, ssh,
// actions, ...).
func filterOutSubtreePlaceholders(rows []registry.Row) []registry.Row {
	out := make([]registry.Row, 0, len(rows))
	for _, r := range rows {
		if !isSubtreePlaceholderRow(r) {
			out = append(out, r)
		}
	}
	return out
}

// subtreeColumns prepends a COMPARTMENT column (each row's
// CompartmentLabel) and makes every other column placeholder-aware: a
// "— loading —" row shows that text in the first column and blanks in the
// rest, rather than reaching into its (nil) Raw the way a real column's
// Get would.
func subtreeColumns(cols []registry.Column) []registry.Column {
	wrapped := make([]registry.Column, len(cols))
	for i, c := range cols {
		i, c := i, c
		wrapped[i] = registry.Column{Header: c.Header, Width: c.Width, Get: func(row registry.Row) string {
			if isSubtreePlaceholderRow(row) {
				if i == 0 {
					return row.Name
				}
				return ""
			}
			return c.Get(row)
		}}
	}
	compCol := registry.Column{Header: "COMPARTMENT", Width: 20, Get: func(row registry.Row) string {
		return row.CompartmentLabel
	}}
	return append([]registry.Column{compCol}, wrapped...)
}
