package app

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/identity"

	"toci/internal/clients"
)

// crumb is one entry in the compartment breadcrumb the user has drilled
// into. The path is tracked as the user navigates rather than fetched
// eagerly as a full tree, so it works regardless of tenancy-level
// "inspect" permissions.
type crumb struct {
	ID   string
	Name string
}

func breadcrumbLabel(path []crumb) string {
	if len(path) == 0 {
		return ""
	}
	s := path[0].Name
	for _, c := range path[1:] {
		s += "/" + c.Name
	}
	return s
}

// rootCompartmentName resolves the tenancy's display name for the
// breadcrumb root. The tenancy OCID is addressable via GetCompartment;
// if that fails (rare permission edge case) it falls back to "root".
func rootCompartmentName(ctx context.Context, factory *clients.Factory, region, tenancyID string) string {
	client, err := factory.Identity(region)
	if err != nil {
		return "root"
	}
	resp, err := client.GetCompartment(ctx, identity.GetCompartmentRequest{CompartmentId: &tenancyID})
	if err != nil || resp.Name == nil {
		return "root"
	}
	return *resp.Name
}

func listRegions(ctx context.Context, factory *clients.Factory, region, tenancyID string) ([]pickerItem, error) {
	client, err := factory.Identity(region)
	if err != nil {
		return nil, err
	}
	resp, err := client.ListRegionSubscriptions(ctx, identity.ListRegionSubscriptionsRequest{TenancyId: &tenancyID})
	if err != nil {
		return nil, err
	}
	items := make([]pickerItem, 0, len(resp.Items))
	for _, r := range resp.Items {
		name := ""
		if r.RegionName != nil {
			name = *r.RegionName
		}
		label := name
		if r.IsHomeRegion != nil && *r.IsHomeRegion {
			label += " (home)"
		}
		items = append(items, pickerItem{key: name, label: label})
	}
	return items, nil
}
