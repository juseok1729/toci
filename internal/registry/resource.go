// Package registry defines the Resource abstraction that every OCI resource
// kind (compartment, VCN, subnet, ...) implements to appear in the TUI.
package registry

import (
	"context"
	"strconv"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

// Scope is the (region, compartment) pair a listing is evaluated against.
// VcnID is an optional extra filter: when set, InstanceResource.List only
// returns instances with a VNIC in that VCN.
type Scope struct {
	Region        string
	CompartmentID string
	VcnID         string
}

// Row is one line in a resource table. Raw holds the original SDK struct so
// the detail view can render it without the resource needing a Detail RPC.
//
// TimeCreated is the resource's own creation timestamp — every List
// implementation below sets it from the underlying SDK struct's own
// TimeCreated field, which OCI exposes on essentially every resource type.
// It's the zero Time if a resource kind genuinely has none. Used to drive
// the "blink recently created rows" feature in internal/app.
type Row struct {
	ID          string
	Name        string
	Raw         any
	TimeCreated time.Time
}

// Column renders one field of a Row into table text. Get is a closure over
// the SDK struct, so field names are checked at compile time.
type Column struct {
	Header string
	Width  int
	Get    func(Row) string
}

// Resource is one browsable OCI resource kind.
type Resource interface {
	// Key identifies the resource for the (future) ":" picker, e.g. "vcn".
	Key() string
	// Label is the human-readable name shown in the UI, e.g. "VCNs".
	Label() string
	Columns() []Column
	// List fetches one page of rows. page is the opaque token from the
	// previous call's return value; pass "" for the first page.
	List(ctx context.Context, s Scope, page string) (rows []Row, nextPage string, err error)
}

// ActionSpec describes one write action a resource offers on a row, e.g.
// starting or stopping an instance.
type ActionSpec struct {
	Key   string // "start", "stop"
	Label string // "Start", "Stop"
}

// Actionable is implemented by resources that support write actions.
// Kept separate from Resource so read-only resources (VCN, Subnet, ...)
// aren't forced to stub it out.
type Actionable interface {
	Actions() []ActionSpec
	RunAction(ctx context.Context, s Scope, key, id string) error
}

// timeOf converts an SDK timestamp pointer to a plain time.Time, zero if
// nil — every List() below uses this to fill Row.TimeCreated from the
// underlying struct's own TimeCreated field.
func timeOf(t *common.SDKTime) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
