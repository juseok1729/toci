package app

import (
	"errors"
	"strings"
	"testing"
)

// fakeServiceError is a minimal common.ServiceError stand-in — the real
// oci-go-sdk one's Error() is a multi-paragraph troubleshooting block,
// exactly what summarizeSubtreeError exists to condense.
type fakeServiceError struct {
	status  int
	code    string
	message string
}

// Error mimics the real SDK's multi-paragraph format — summarizeSubtreeError
// must never use this directly for a ServiceError; it reads the typed
// GetHTTPStatusCode/GetCode/GetMessage accessors below instead.
func (e fakeServiceError) Error() string {
	return "Error returned by Core Service. Message: " + e.message + "\nOperation Name: ListRouteTables\nTimestamp: 2026-01-01T00:00:00Z"
}
func (e fakeServiceError) GetHTTPStatusCode() int  { return e.status }
func (e fakeServiceError) GetMessage() string      { return e.message }
func (e fakeServiceError) GetCode() string         { return e.code }
func (e fakeServiceError) GetOpcRequestID() string { return "req-1" }

// TestSummarizeSubtreeErrorCondensesServiceError guards a reported bug: a
// ServiceError's own Error() is the OCI Go SDK's full multi-paragraph
// troubleshooting block (newlines included) — feeding that straight into
// m.statusMsg broke the one-line status bar into several lines of raw SDK
// text. summarizeSubtreeError must reduce it to one line: status, code,
// and the actual message, nothing else.
func TestSummarizeSubtreeErrorCondensesServiceError(t *testing.T) {
	err := fakeServiceError{status: 400, code: "LimitExceeded", message: "Too many requests"}
	got := summarizeSubtreeError(err)

	if strings.Contains(got, "\n") {
		t.Errorf("summarizeSubtreeError() = %q, contains a newline (would still break the status bar)", got)
	}
	for _, want := range []string{"400", "LimitExceeded", "Too many requests"} {
		if !strings.Contains(got, want) {
			t.Errorf("summarizeSubtreeError() = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "Operation Name") || strings.Contains(got, "Timestamp") {
		t.Errorf("summarizeSubtreeError() = %q, should not carry the SDK's troubleshooting boilerplate", got)
	}
}

// TestSummarizeSubtreeErrorTruncatesMultilinePlainError covers the
// non-ServiceError fallback path (a plain Go error — network/context/etc.)
// with the same defensive newline guard.
func TestSummarizeSubtreeErrorTruncatesMultilinePlainError(t *testing.T) {
	err := errors.New("dial tcp: connection refused\nretrying...")
	if got, want := summarizeSubtreeError(err), "dial tcp: connection refused"; got != want {
		t.Errorf("summarizeSubtreeError() = %q, want %q", got, want)
	}
}
