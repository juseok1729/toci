package registry

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRecentSource struct {
	key, label, state string
	rows              []Row
	err               error
}

func (f fakeRecentSource) Key() string   { return f.key }
func (f fakeRecentSource) Label() string { return f.label }
func (f fakeRecentSource) Columns() []Column {
	return []Column{{Header: "STATE", Get: func(Row) string { return f.state }}}
}
func (f fakeRecentSource) List(context.Context, Scope, string) ([]Row, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	return f.rows, "", nil
}

func TestNewRecentResourceExcludesNonListableAndDuplicateKinds(t *testing.T) {
	all := []Resource{
		fakeRecentSource{key: "compartment"},
		fakeRecentSource{key: "igw"},
		fakeRecentSource{key: "nat-gateway"},
		fakeRecentSource{key: "service-gateway"},
		fakeRecentSource{key: "gateway"},
		fakeRecentSource{key: "drg-route-table"},
		fakeRecentSource{key: "drg-route-distribution"},
		fakeRecentSource{key: "instance"},
	}

	r := NewRecentResource(all)
	got := make(map[string]bool, len(r.sources))
	for _, s := range r.sources {
		got[s.Key()] = true
	}
	for _, excluded := range []string{"compartment", "igw", "nat-gateway", "service-gateway", "drg-route-table", "drg-route-distribution"} {
		if got[excluded] {
			t.Errorf("expected %q to be excluded from RecentResource's sources", excluded)
		}
	}
	for _, kept := range []string{"gateway", "instance"} {
		if !got[kept] {
			t.Errorf("expected %q to be kept in RecentResource's sources", kept)
		}
	}
}

func TestRecentResourceListMergesSortsAndFiltersByWindow(t *testing.T) {
	now := time.Now()
	outsideWindow := now.Add(-4 * 24 * time.Hour) // RecentWindow is 3 days
	older := now.Add(-time.Hour)
	newer := now.Add(-time.Minute)

	sources := []Resource{
		fakeRecentSource{key: "instance", label: "Instances", state: "RUNNING", rows: []Row{
			{ID: "i1", Name: "old-instance", TimeCreated: outsideWindow},
			{ID: "i2", Name: "recent-instance", TimeCreated: older},
		}},
		fakeRecentSource{key: "vcn", label: "VCNs", rows: []Row{
			{ID: "v1", Name: "new-vcn", TimeCreated: newer},
		}},
		fakeRecentSource{key: "broken", label: "Broken", err: errors.New("boom")},
	}

	r := NewRecentResource(sources)
	rows, next, err := r.List(context.Background(), Scope{}, "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if next != "" {
		t.Errorf("next page = %q, want \"\" (fully drained in one call)", next)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (old-instance outside RecentWindow, broken source skipped): %+v", len(rows), rows)
	}
	if rows[0].Name != "new-vcn" || rows[1].Name != "recent-instance" {
		t.Errorf("rows not sorted newest-first: got %q, %q", rows[0].Name, rows[1].Name)
	}
	if got := rows[1].Raw.(recentRow).State; got != "RUNNING" {
		t.Errorf("State = %q, want RUNNING (read via the source's own STATE column)", got)
	}
	if got := rows[1].Raw.(recentRow).Kind; got != "Instances" {
		t.Errorf("Kind = %q, want Instances (source's Label())", got)
	}
}

func TestRecentResourceColumnsReadRecentRow(t *testing.T) {
	r := NewRecentResource(nil)
	created := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	row := Row{TimeCreated: created, Raw: recentRow{Kind: "Instances", Name: "my-instance", State: "RUNNING"}}

	want := map[string]string{
		"KIND":  "Instances",
		"NAME":  "my-instance",
		"STATE": "RUNNING",
	}
	for _, col := range r.Columns() {
		if col.Header == "CREATED" {
			continue // formatted in local time, not worth pinning down here
		}
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}
