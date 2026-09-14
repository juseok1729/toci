package registry

import (
	"context"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/filestorage"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"toci/internal/clients"
)

type FileSystemResource struct {
	factory *clients.Factory
}

func NewFileSystemResource(f *clients.Factory) *FileSystemResource {
	return &FileSystemResource{factory: f}
}

func (r *FileSystemResource) Key() string   { return "file-system" }
func (r *FileSystemResource) Label() string { return "File Systems" }

func (r *FileSystemResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(filestorage.FileSystemSummary).DisplayName)
		}},
		{Header: "STATE", Width: 12, Get: func(row Row) string {
			return stateLabel(row.Raw.(filestorage.FileSystemSummary).LifecycleState)
		}},
		{Header: "AD", Width: 16, Get: func(row Row) string {
			return deref(row.Raw.(filestorage.FileSystemSummary).AvailabilityDomain)
		}},
		{Header: "SIZE(GB)", Width: 10, Get: func(row Row) string {
			mb := row.Raw.(filestorage.FileSystemSummary).MeteredBytes
			if mb == nil {
				return ""
			}
			return fmt.Sprintf("%.1f", float64(*mb)/(1<<30))
		}},
	}
}

// List fetches every file system in the compartment. Unlike every other
// resource here, ListFileSystems takes a mandatory AvailabilityDomain — one
// call can't span the whole region — so this enumerates the region's ADs
// (ListAvailabilityDomains; AD list is tenancy-wide, the same regardless of
// which compartment ID is passed) and fully drains each one's own paginated
// list internally, returning everything in one call rather than delegating
// page-by-page to the caller the way every other resource's List() does.
func (r *FileSystemResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	idClient, err := r.factory.Identity(s.Region)
	if err != nil {
		return nil, "", err
	}
	adResp, err := idClient.ListAvailabilityDomains(ctx, identity.ListAvailabilityDomainsRequest{CompartmentId: &s.CompartmentID})
	if err != nil {
		return nil, "", err
	}

	client, err := r.factory.FileStorage(s.Region)
	if err != nil {
		return nil, "", err
	}

	var rows []Row
	for _, ad := range adResp.Items {
		if ad.Name == nil {
			continue
		}
		adName := *ad.Name

		fsPage := ""
		for {
			req := filestorage.ListFileSystemsRequest{CompartmentId: &s.CompartmentID, AvailabilityDomain: &adName}
			if fsPage != "" {
				req.Page = &fsPage
			}
			resp, err := client.ListFileSystems(ctx, req)
			if err != nil {
				return nil, "", err
			}
			for _, fs := range resp.Items {
				rows = append(rows, Row{ID: deref(fs.Id), Name: deref(fs.DisplayName), TimeCreated: timeOf(fs.TimeCreated), Raw: fs})
			}
			if resp.OpcNextPage == nil {
				break
			}
			fsPage = *resp.OpcNextPage
		}
	}
	return rows, "", nil
}
