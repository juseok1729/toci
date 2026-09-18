package registry

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/loadbalancer"
	"toci/internal/clients"
)

// errSelectLbFirst is every LB-scoped resource's List error when s.LbID is
// empty — none of them (Listeners, Backend Sets, Certificates, Hostnames,
// Path Route Sets, Rule Sets, Routing Policies) have a "list across every
// load balancer in the compartment" concept the way, say, Node Pools sort
// of do; they're only ever reachable scoped (see model.selectLbFilter).
var errSelectLbFirst = errors.New("select a load balancer first (\"i\" or Enter on a lb row)")

// getScopedLoadBalancer fetches the full LoadBalancer object s.LbID points
// at — every sub-resource below (Listeners, BackendSets, ...) is a plain
// map field already nested on this one struct (unlike, say, DrgRouteTable,
// which needs its own List call), so one GetLoadBalancer covers all seven
// LB-scoped resource kinds.
func getScopedLoadBalancer(ctx context.Context, factory *clients.Factory, region, lbID string) (loadbalancer.LoadBalancer, error) {
	client, err := factory.LoadBalancer(region)
	if err != nil {
		return loadbalancer.LoadBalancer{}, err
	}
	resp, err := client.GetLoadBalancer(ctx, loadbalancer.GetLoadBalancerRequest{LoadBalancerId: &lbID})
	if err != nil {
		return loadbalancer.LoadBalancer{}, err
	}
	return resp.LoadBalancer, nil
}

// sortedMapKeys returns m's keys sorted — every LB sub-resource comes back
// as a map (name -> value), and Go's map iteration order is random, so
// without this the table would reshuffle its rows on every reload.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// intPtrString is itoa's *int counterpart — several LB sub-resource fields
// (Listener.Port, HealthChecker.Port) are optional pointers rather than
// instance.go's *int64 fields, which int64String already covers.
func intPtrString(n *int) string {
	if n == nil {
		return ""
	}
	return itoa(*n)
}

// --- Listeners ---

type LbListenerResource struct {
	factory *clients.Factory
}

func NewLbListenerResource(f *clients.Factory) *LbListenerResource {
	return &LbListenerResource{factory: f}
}

func (r *LbListenerResource) Key() string   { return "lb-listener" }
func (r *LbListenerResource) Label() string { return "Listeners" }

func (r *LbListenerResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 24, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.Listener).Name)
		}},
		{Header: "PORT", Width: 6, Get: func(row Row) string {
			return intPtrString(row.Raw.(loadbalancer.Listener).Port)
		}},
		{Header: "PROTOCOL", Width: 10, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.Listener).Protocol)
		}},
		{Header: "BACKEND SET", Width: 24, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.Listener).DefaultBackendSetName)
		}},
		{Header: "HOSTNAMES", Width: 24, Get: func(row Row) string {
			return strings.Join(row.Raw.(loadbalancer.Listener).HostnameNames, ", ")
		}},
		{Header: "ROUTING POLICY", Width: 20, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.Listener).RoutingPolicyName)
		}},
		{Header: "RULE SETS", Width: 20, Get: func(row Row) string {
			return strings.Join(row.Raw.(loadbalancer.Listener).RuleSetNames, ", ")
		}},
	}
}

func (r *LbListenerResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	if s.LbID == "" {
		return nil, "", errSelectLbFirst
	}
	lb, err := getScopedLoadBalancer(ctx, r.factory, s.Region, s.LbID)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(lb.Listeners))
	for _, name := range sortedMapKeys(lb.Listeners) {
		l := lb.Listeners[name]
		rows = append(rows, Row{ID: name, Name: name, Raw: l})
	}
	return rows, "", nil
}

// --- Backend Sets ---

type LbBackendSetResource struct {
	factory *clients.Factory
}

func NewLbBackendSetResource(f *clients.Factory) *LbBackendSetResource {
	return &LbBackendSetResource{factory: f}
}

func (r *LbBackendSetResource) Key() string   { return "lb-backend-set" }
func (r *LbBackendSetResource) Label() string { return "Backend Sets" }

func (r *LbBackendSetResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 24, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.BackendSet).Name)
		}},
		{Header: "POLICY", Width: 18, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.BackendSet).Policy)
		}},
		{Header: "BACKENDS", Width: 10, Get: func(row Row) string {
			return itoa(len(row.Raw.(loadbalancer.BackendSet).Backends))
		}},
		{Header: "HEALTH CHECK", Width: 20, Get: func(row Row) string {
			hc := row.Raw.(loadbalancer.BackendSet).HealthChecker
			if hc == nil {
				return ""
			}
			return deref(hc.Protocol) + ":" + intPtrString(hc.Port) + deref(hc.UrlPath)
		}},
	}
}

func (r *LbBackendSetResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	if s.LbID == "" {
		return nil, "", errSelectLbFirst
	}
	lb, err := getScopedLoadBalancer(ctx, r.factory, s.Region, s.LbID)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(lb.BackendSets))
	for _, name := range sortedMapKeys(lb.BackendSets) {
		bs := lb.BackendSets[name]
		rows = append(rows, Row{ID: name, Name: name, Raw: bs})
	}
	return rows, "", nil
}

// --- Certificates ---

type LbCertificateResource struct {
	factory *clients.Factory
}

func NewLbCertificateResource(f *clients.Factory) *LbCertificateResource {
	return &LbCertificateResource{factory: f}
}

func (r *LbCertificateResource) Key() string   { return "lb-certificate" }
func (r *LbCertificateResource) Label() string { return "Certificates" }

func (r *LbCertificateResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.Certificate).CertificateName)
		}},
		{Header: "CA CERT", Width: 10, Get: func(row Row) string {
			if row.Raw.(loadbalancer.Certificate).CaCertificate != nil {
				return "Yes"
			}
			return "No"
		}},
	}
}

func (r *LbCertificateResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	if s.LbID == "" {
		return nil, "", errSelectLbFirst
	}
	lb, err := getScopedLoadBalancer(ctx, r.factory, s.Region, s.LbID)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(lb.Certificates))
	for _, name := range sortedMapKeys(lb.Certificates) {
		c := lb.Certificates[name]
		rows = append(rows, Row{ID: name, Name: name, Raw: c})
	}
	return rows, "", nil
}

// --- Hostnames ---

type LbHostnameResource struct {
	factory *clients.Factory
}

func NewLbHostnameResource(f *clients.Factory) *LbHostnameResource {
	return &LbHostnameResource{factory: f}
}

func (r *LbHostnameResource) Key() string   { return "lb-hostname" }
func (r *LbHostnameResource) Label() string { return "Hostnames" }

func (r *LbHostnameResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 24, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.Hostname).Name)
		}},
		{Header: "HOSTNAME", Width: 40, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.Hostname).Hostname)
		}},
	}
}

func (r *LbHostnameResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	if s.LbID == "" {
		return nil, "", errSelectLbFirst
	}
	lb, err := getScopedLoadBalancer(ctx, r.factory, s.Region, s.LbID)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(lb.Hostnames))
	for _, name := range sortedMapKeys(lb.Hostnames) {
		h := lb.Hostnames[name]
		rows = append(rows, Row{ID: name, Name: name, Raw: h})
	}
	return rows, "", nil
}

// --- Path Route Sets ---

type LbPathRouteSetResource struct {
	factory *clients.Factory
}

func NewLbPathRouteSetResource(f *clients.Factory) *LbPathRouteSetResource {
	return &LbPathRouteSetResource{factory: f}
}

func (r *LbPathRouteSetResource) Key() string   { return "lb-path-route-set" }
func (r *LbPathRouteSetResource) Label() string { return "Path Route Sets" }

func (r *LbPathRouteSetResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.PathRouteSet).Name)
		}},
		{Header: "ROUTES", Width: 10, Get: func(row Row) string {
			return itoa(len(row.Raw.(loadbalancer.PathRouteSet).PathRoutes))
		}},
	}
}

func (r *LbPathRouteSetResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	if s.LbID == "" {
		return nil, "", errSelectLbFirst
	}
	lb, err := getScopedLoadBalancer(ctx, r.factory, s.Region, s.LbID)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(lb.PathRouteSets))
	for _, name := range sortedMapKeys(lb.PathRouteSets) {
		p := lb.PathRouteSets[name]
		rows = append(rows, Row{ID: name, Name: name, Raw: p})
	}
	return rows, "", nil
}

// --- Rule Sets ---

type LbRuleSetResource struct {
	factory *clients.Factory
}

func NewLbRuleSetResource(f *clients.Factory) *LbRuleSetResource {
	return &LbRuleSetResource{factory: f}
}

func (r *LbRuleSetResource) Key() string   { return "lb-rule-set" }
func (r *LbRuleSetResource) Label() string { return "Rule Sets" }

func (r *LbRuleSetResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.RuleSet).Name)
		}},
		{Header: "RULES", Width: 10, Get: func(row Row) string {
			return itoa(len(row.Raw.(loadbalancer.RuleSet).Items))
		}},
	}
}

func (r *LbRuleSetResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	if s.LbID == "" {
		return nil, "", errSelectLbFirst
	}
	lb, err := getScopedLoadBalancer(ctx, r.factory, s.Region, s.LbID)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(lb.RuleSets))
	for _, name := range sortedMapKeys(lb.RuleSets) {
		rs := lb.RuleSets[name]
		rows = append(rows, Row{ID: name, Name: name, Raw: rs})
	}
	return rows, "", nil
}

// --- Routing Policies ---

type LbRoutingPolicyResource struct {
	factory *clients.Factory
}

func NewLbRoutingPolicyResource(f *clients.Factory) *LbRoutingPolicyResource {
	return &LbRoutingPolicyResource{factory: f}
}

func (r *LbRoutingPolicyResource) Key() string   { return "lb-routing-policy" }
func (r *LbRoutingPolicyResource) Label() string { return "Routing Policies" }

func (r *LbRoutingPolicyResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(loadbalancer.RoutingPolicy).Name)
		}},
		{Header: "RULES", Width: 10, Get: func(row Row) string {
			return itoa(len(row.Raw.(loadbalancer.RoutingPolicy).Rules))
		}},
		{Header: "CONDITION LANG", Width: 16, Get: func(row Row) string {
			return string(row.Raw.(loadbalancer.RoutingPolicy).ConditionLanguageVersion)
		}},
	}
}

func (r *LbRoutingPolicyResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	if s.LbID == "" {
		return nil, "", errSelectLbFirst
	}
	lb, err := getScopedLoadBalancer(ctx, r.factory, s.Region, s.LbID)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(lb.RoutingPolicies))
	for _, name := range sortedMapKeys(lb.RoutingPolicies) {
		p := lb.RoutingPolicies[name]
		rows = append(rows, Row{ID: name, Name: name, Raw: p})
	}
	return rows, "", nil
}
