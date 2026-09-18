package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/loadbalancer"
)

// TestLbScopedResourcesRequireLbID checks every LB-scoped resource's List
// guard: none of them (Listeners, Backend Sets, Certificates, Hostnames,
// Path Route Sets, Rule Sets, Routing Policies) have an unscoped "list
// across every load balancer" concept, so an empty Scope.LbID must fail
// fast with errSelectLbFirst instead of reaching for a client at all
// (which would panic here — every resource below is built with a nil
// factory).
func TestLbScopedResourcesRequireLbID(t *testing.T) {
	resources := []Resource{
		NewLbListenerResource(nil),
		NewLbBackendSetResource(nil),
		NewLbCertificateResource(nil),
		NewLbHostnameResource(nil),
		NewLbPathRouteSetResource(nil),
		NewLbRuleSetResource(nil),
		NewLbRoutingPolicyResource(nil),
	}
	for _, r := range resources {
		_, _, err := r.List(context.Background(), Scope{}, "")
		if !errors.Is(err, errSelectLbFirst) {
			t.Errorf("%s.List with empty LbID: err = %v, want errSelectLbFirst", r.Key(), err)
		}
	}
}

func TestSortedMapKeysReturnsSortedOrder(t *testing.T) {
	m := map[string]int{"charlie": 3, "alpha": 1, "bravo": 2}
	got := sortedMapKeys(m)
	want := []string{"alpha", "bravo", "charlie"}
	if len(got) != len(want) {
		t.Fatalf("sortedMapKeys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortedMapKeys = %v, want %v", got, want)
			break
		}
	}
}

func TestIntPtrString(t *testing.T) {
	if got := intPtrString(nil); got != "" {
		t.Errorf("intPtrString(nil) = %q, want empty", got)
	}
	n := 8080
	if got := intPtrString(&n); got != "8080" {
		t.Errorf("intPtrString(&8080) = %q, want %q", got, "8080")
	}
}

func TestLbListenerResourceColumns(t *testing.T) {
	r := NewLbListenerResource(nil)
	port := 443
	row := Row{Raw: loadbalancer.Listener{
		Name:                  common.String("https-listener"),
		Port:                  &port,
		Protocol:              common.String("HTTP"),
		DefaultBackendSetName: common.String("web-backends"),
		HostnameNames:         []string{"app.example.com", "api.example.com"},
		RoutingPolicyName:     common.String("main-policy"),
		RuleSetNames:          []string{"add-headers"},
	}}
	want := map[string]string{
		"NAME":           "https-listener",
		"PORT":           "443",
		"PROTOCOL":       "HTTP",
		"BACKEND SET":    "web-backends",
		"HOSTNAMES":      "app.example.com, api.example.com",
		"ROUTING POLICY": "main-policy",
		"RULE SETS":      "add-headers",
	}
	for _, col := range r.Columns() {
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}

func TestLbBackendSetResourceColumns(t *testing.T) {
	r := NewLbBackendSetResource(nil)
	hcPort := 80
	row := Row{Raw: loadbalancer.BackendSet{
		Name:     common.String("web-backends"),
		Policy:   common.String("LEAST_CONNECTIONS"),
		Backends: []loadbalancer.Backend{{}, {}},
		HealthChecker: &loadbalancer.HealthChecker{
			Protocol: common.String("HTTP"),
			Port:     &hcPort,
			UrlPath:  common.String("/health"),
		},
	}}
	want := map[string]string{
		"NAME":         "web-backends",
		"POLICY":       "LEAST_CONNECTIONS",
		"BACKENDS":     "2",
		"HEALTH CHECK": "HTTP:80/health",
	}
	for _, col := range r.Columns() {
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}

// TestLbBackendSetResourceColumnsNilHealthChecker guards the nil
// HealthChecker case — mandatory per the SDK's own doc, but every other
// field here is read through the same kind of pointer without a nil
// check, so this is worth locking in explicitly.
func TestLbBackendSetResourceColumnsNilHealthChecker(t *testing.T) {
	r := NewLbBackendSetResource(nil)
	row := Row{Raw: loadbalancer.BackendSet{Name: common.String("no-health-checker")}}
	for _, col := range r.Columns() {
		if col.Header == "HEALTH CHECK" {
			if got := col.Get(row); got != "" {
				t.Errorf("HEALTH CHECK with nil HealthChecker = %q, want empty", got)
			}
		}
	}
}

func TestLbCertificateResourceColumns(t *testing.T) {
	r := NewLbCertificateResource(nil)
	withCA := Row{Raw: loadbalancer.Certificate{CertificateName: common.String("cert-with-ca"), CaCertificate: common.String("-----BEGIN...")}}
	withoutCA := Row{Raw: loadbalancer.Certificate{CertificateName: common.String("cert-no-ca")}}

	want := map[string]map[string]string{
		"cert-with-ca": {"NAME": "cert-with-ca", "CA CERT": "Yes"},
		"cert-no-ca":   {"NAME": "cert-no-ca", "CA CERT": "No"},
	}
	for _, row := range []Row{withCA, withoutCA} {
		name := row.Raw.(loadbalancer.Certificate).CertificateName
		for _, col := range r.Columns() {
			if got, want := col.Get(row), want[*name][col.Header]; got != want {
				t.Errorf("%s column %q = %q, want %q", *name, col.Header, got, want)
			}
		}
	}
}

func TestLbHostnameResourceColumns(t *testing.T) {
	r := NewLbHostnameResource(nil)
	row := Row{Raw: loadbalancer.Hostname{Name: common.String("app-hostname"), Hostname: common.String("app.example.com")}}
	want := map[string]string{"NAME": "app-hostname", "HOSTNAME": "app.example.com"}
	for _, col := range r.Columns() {
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}

func TestLbPathRouteSetResourceColumns(t *testing.T) {
	r := NewLbPathRouteSetResource(nil)
	row := Row{Raw: loadbalancer.PathRouteSet{
		Name:       common.String("legacy-routes"),
		PathRoutes: []loadbalancer.PathRoute{{}, {}, {}},
	}}
	want := map[string]string{"NAME": "legacy-routes", "ROUTES": "3"}
	for _, col := range r.Columns() {
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}

func TestLbRuleSetResourceColumns(t *testing.T) {
	r := NewLbRuleSetResource(nil)
	row := Row{Raw: loadbalancer.RuleSet{
		Name:  common.String("add-headers"),
		Items: []loadbalancer.Rule{nil, nil},
	}}
	want := map[string]string{"NAME": "add-headers", "RULES": "2"}
	for _, col := range r.Columns() {
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}

func TestLbRoutingPolicyResourceColumns(t *testing.T) {
	r := NewLbRoutingPolicyResource(nil)
	row := Row{Raw: loadbalancer.RoutingPolicy{
		Name:                     common.String("main-policy"),
		ConditionLanguageVersion: loadbalancer.RoutingPolicyConditionLanguageVersionV1,
		Rules:                    []loadbalancer.RoutingRule{{}},
	}}
	want := map[string]string{"NAME": "main-policy", "RULES": "1", "CONDITION LANG": "V1"}
	for _, col := range r.Columns() {
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}
