package clients

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
)

// fakeClient stands in for a generated OCI client in these tests — it
// embeds common.BaseClient exactly like every real one does, so
// *fakeClient satisfies retryConfigurable the same way get() relies on
// for the real ones, without needing a real signer/config provider.
type fakeClient struct {
	common.BaseClient
}

// TestGetConfiguresRetryPolicy guards a reported bug: none of the SDK's
// generated List/Get/etc. calls retry anything by default (each one falls
// back to common.NoRetryPolicy() unless the client itself overrides it),
// so a transient 429 from OCI's per-second rate limit — easy to trip
// during the compartment subtree fan-out's many concurrent List calls —
// surfaced as a hard failure. get() must configure a retry policy on
// every client it builds.
func TestGetConfiguresRetryPolicy(t *testing.T) {
	f := &Factory{cache: map[string]any{}}
	client, err := get(f, "us-ashburn-1", "fake", func() (fakeClient, error) {
		return fakeClient{}, nil
	})
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if client.RetryPolicy() == nil {
		t.Error("get() did not configure a retry policy on the built client")
	}
}

func TestGetCachesClientPerRegionAndKind(t *testing.T) {
	f := &Factory{cache: map[string]any{}}
	calls := 0
	build := func() (fakeClient, error) {
		calls++
		return fakeClient{}, nil
	}
	if _, err := get(f, "us-ashburn-1", "fake", build); err != nil {
		t.Fatal(err)
	}
	if _, err := get(f, "us-ashburn-1", "fake", build); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("build() called %d times, want 1 (the second get should hit the cache)", calls)
	}
}
