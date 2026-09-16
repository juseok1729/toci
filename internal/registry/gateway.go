package registry

import (
	"context"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/core"
	"toci/internal/clients"
)

// --- Internet Gateway ---

type InternetGatewayResource struct {
	factory *clients.Factory
}

func NewInternetGatewayResource(f *clients.Factory) *InternetGatewayResource {
	return &InternetGatewayResource{factory: f}
}

func (r *InternetGatewayResource) Key() string   { return "igw" }
func (r *InternetGatewayResource) Label() string { return "Internet Gateways" }

func (r *InternetGatewayResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.InternetGateway).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.InternetGateway).LifecycleState)
		}},
		{Header: "ENABLED", Width: 10, Get: func(row Row) string {
			return yesNo(row.Raw.(core.InternetGateway).IsEnabled)
		}},
	}
}

func (r *InternetGatewayResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}
	req := core.ListInternetGatewaysRequest{CompartmentId: &s.CompartmentID}
	if s.VcnID != "" {
		req.VcnId = &s.VcnID
	}
	if page != "" {
		req.Page = &page
	}
	resp, err := client.ListInternetGateways(ctx, req)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(resp.Items))
	for _, ig := range resp.Items {
		rows = append(rows, Row{ID: deref(ig.Id), Name: deref(ig.DisplayName), TimeCreated: timeOf(ig.TimeCreated), Raw: ig})
	}
	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}

// --- NAT Gateway ---

type NatGatewayResource struct {
	factory *clients.Factory
}

func NewNatGatewayResource(f *clients.Factory) *NatGatewayResource {
	return &NatGatewayResource{factory: f}
}

func (r *NatGatewayResource) Key() string   { return "nat-gateway" }
func (r *NatGatewayResource) Label() string { return "NAT Gateways" }

func (r *NatGatewayResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.NatGateway).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.NatGateway).LifecycleState)
		}},
		{Header: "NAT IP", Width: 16, Get: func(row Row) string {
			return deref(row.Raw.(core.NatGateway).NatIp)
		}},
		{Header: "BLOCKED", Width: 10, Get: func(row Row) string {
			return yesNo(row.Raw.(core.NatGateway).BlockTraffic)
		}},
	}
}

func (r *NatGatewayResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}
	req := core.ListNatGatewaysRequest{CompartmentId: &s.CompartmentID}
	if s.VcnID != "" {
		req.VcnId = &s.VcnID
	}
	if page != "" {
		req.Page = &page
	}
	resp, err := client.ListNatGateways(ctx, req)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(resp.Items))
	for _, ng := range resp.Items {
		rows = append(rows, Row{ID: deref(ng.Id), Name: deref(ng.DisplayName), TimeCreated: timeOf(ng.TimeCreated), Raw: ng})
	}
	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}

// --- Service Gateway ---

type ServiceGatewayResource struct {
	factory *clients.Factory
}

func NewServiceGatewayResource(f *clients.Factory) *ServiceGatewayResource {
	return &ServiceGatewayResource{factory: f}
}

func (r *ServiceGatewayResource) Key() string   { return "service-gateway" }
func (r *ServiceGatewayResource) Label() string { return "Service Gateways" }

func (r *ServiceGatewayResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(core.ServiceGateway).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(core.ServiceGateway).LifecycleState)
		}},
		{Header: "SERVICES", Width: 10, Get: func(row Row) string {
			return itoa(len(row.Raw.(core.ServiceGateway).Services))
		}},
		{Header: "BLOCKED", Width: 10, Get: func(row Row) string {
			return yesNo(row.Raw.(core.ServiceGateway).BlockTraffic)
		}},
	}
}

func (r *ServiceGatewayResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}
	req := core.ListServiceGatewaysRequest{CompartmentId: &s.CompartmentID}
	if s.VcnID != "" {
		req.VcnId = &s.VcnID
	}
	if page != "" {
		req.Page = &page
	}
	resp, err := client.ListServiceGateways(ctx, req)
	if err != nil {
		return nil, "", err
	}
	rows := make([]Row, 0, len(resp.Items))
	for _, sg := range resp.Items {
		rows = append(rows, Row{ID: deref(sg.Id), Name: deref(sg.DisplayName), TimeCreated: timeOf(sg.TimeCreated), Raw: sg})
	}
	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}

// --- Gateways (combined) ---

// gatewayRow is the combined "Gateways" view's row shape. Internet/NAT/
// Service gateways come from three different SDK types with different
// fields, normalized into one common shape here rather than a type switch
// inside every column's Get closure.
type gatewayRow struct {
	Type   string
	Name   string
	State  string
	Detail string
}

type GatewayResource struct {
	factory *clients.Factory
}

func NewGatewayResource(f *clients.Factory) *GatewayResource {
	return &GatewayResource{factory: f}
}

func (r *GatewayResource) Key() string   { return "gateway" }
func (r *GatewayResource) Label() string { return "All Gateways" }

func (r *GatewayResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string { return row.Raw.(gatewayRow).Name }},
		{Header: "TYPE", Width: 10, Get: func(row Row) string { return row.Raw.(gatewayRow).Type }},
		{Header: "STATE", Width: 12, Get: func(row Row) string { return row.Raw.(gatewayRow).State }},
		{Header: "DETAIL", Width: 20, Get: func(row Row) string { return row.Raw.(gatewayRow).Detail }},
	}
}

// List fetches every Internet/NAT/Service gateway in the compartment (and
// VCN, if scoped) and merges them into one list for a single "everything
// at once" view. Unlike every other resource here (except FileSystemResource,
// for its own reason), this drains all three underlying lists fully in one
// call rather than delegating page-by-page to the caller — there's no
// single page token that could represent "which of the three lists, and
// which page of it".
func (r *GatewayResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.VirtualNetwork(s.Region)
	if err != nil {
		return nil, "", err
	}

	var rows []Row

	igPage := ""
	for {
		req := core.ListInternetGatewaysRequest{CompartmentId: &s.CompartmentID}
		if s.VcnID != "" {
			req.VcnId = &s.VcnID
		}
		if igPage != "" {
			req.Page = &igPage
		}
		resp, err := client.ListInternetGateways(ctx, req)
		if err != nil {
			return nil, "", err
		}
		for _, ig := range resp.Items {
			detail := "Disabled"
			if ig.IsEnabled != nil && *ig.IsEnabled {
				detail = "Enabled"
			}
			rows = append(rows, Row{
				ID: deref(ig.Id), Name: deref(ig.DisplayName), TimeCreated: timeOf(ig.TimeCreated),
				Raw: gatewayRow{Type: "Internet", Name: deref(ig.DisplayName), State: stateLabel(ig.LifecycleState), Detail: detail},
			})
		}
		if resp.OpcNextPage == nil {
			break
		}
		igPage = *resp.OpcNextPage
	}

	natPage := ""
	for {
		req := core.ListNatGatewaysRequest{CompartmentId: &s.CompartmentID}
		if s.VcnID != "" {
			req.VcnId = &s.VcnID
		}
		if natPage != "" {
			req.Page = &natPage
		}
		resp, err := client.ListNatGateways(ctx, req)
		if err != nil {
			return nil, "", err
		}
		for _, ng := range resp.Items {
			detail := deref(ng.NatIp)
			if ng.BlockTraffic != nil && *ng.BlockTraffic {
				detail = "Blocked"
			}
			rows = append(rows, Row{
				ID: deref(ng.Id), Name: deref(ng.DisplayName), TimeCreated: timeOf(ng.TimeCreated),
				Raw: gatewayRow{Type: "NAT", Name: deref(ng.DisplayName), State: stateLabel(ng.LifecycleState), Detail: detail},
			})
		}
		if resp.OpcNextPage == nil {
			break
		}
		natPage = *resp.OpcNextPage
	}

	sgPage := ""
	for {
		req := core.ListServiceGatewaysRequest{CompartmentId: &s.CompartmentID}
		if s.VcnID != "" {
			req.VcnId = &s.VcnID
		}
		if sgPage != "" {
			req.Page = &sgPage
		}
		resp, err := client.ListServiceGateways(ctx, req)
		if err != nil {
			return nil, "", err
		}
		for _, sg := range resp.Items {
			detail := fmt.Sprintf("%d service(s)", len(sg.Services))
			if sg.BlockTraffic != nil && *sg.BlockTraffic {
				detail = "Blocked"
			}
			rows = append(rows, Row{
				ID: deref(sg.Id), Name: deref(sg.DisplayName), TimeCreated: timeOf(sg.TimeCreated),
				Raw: gatewayRow{Type: "Service", Name: deref(sg.DisplayName), State: stateLabel(sg.LifecycleState), Detail: detail},
			})
		}
		if resp.OpcNextPage == nil {
			break
		}
		sgPage = *resp.OpcNextPage
	}

	return rows, "", nil
}

// yesNo renders an optional bool as "Yes"/"No" — nil (field not returned)
// reads as "No", same as OCI's own default for these gateway flags.
func yesNo(b *bool) string {
	if b != nil && *b {
		return "Yes"
	}
	return "No"
}
