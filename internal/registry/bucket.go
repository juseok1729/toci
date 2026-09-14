package registry

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/objectstorage"
	"toci/internal/clients"
)

type BucketResource struct {
	factory *clients.Factory
}

func NewBucketResource(f *clients.Factory) *BucketResource {
	return &BucketResource{factory: f}
}

func (r *BucketResource) Key() string   { return "bucket" }
func (r *BucketResource) Label() string { return "Buckets" }

func (r *BucketResource) Columns() []Column {
	return []Column{
		{Header: "NAME", Width: 30, Get: func(row Row) string {
			return deref(row.Raw.(objectstorage.BucketSummary).Name)
		}},
		{Header: "NAMESPACE", Width: 20, Get: func(row Row) string {
			return deref(row.Raw.(objectstorage.BucketSummary).Namespace)
		}},
	}
}

// List fetches every bucket in the compartment. Buckets are addressed by
// (namespace, name), not an OCID — ListBuckets needs the tenancy's Object
// Storage namespace up front, so this resolves it (GetNamespace) before
// paginating, on every call rather than caching it — one extra lightweight
// call per page isn't worth adding state for.
func (r *BucketResource) List(ctx context.Context, s Scope, page string) ([]Row, string, error) {
	client, err := r.factory.ObjectStorage(s.Region)
	if err != nil {
		return nil, "", err
	}

	nsResp, err := client.GetNamespace(ctx, objectstorage.GetNamespaceRequest{CompartmentId: &s.CompartmentID})
	if err != nil {
		return nil, "", err
	}

	req := objectstorage.ListBucketsRequest{NamespaceName: nsResp.Value, CompartmentId: &s.CompartmentID}
	if page != "" {
		req.Page = &page
	}

	resp, err := client.ListBuckets(ctx, req)
	if err != nil {
		return nil, "", err
	}

	rows := make([]Row, 0, len(resp.Items))
	for _, b := range resp.Items {
		// Buckets have no OCID — ID is the (namespace, name) pair that
		// actually addresses one, same as every Object Storage API call.
		rows = append(rows, Row{ID: deref(b.Namespace) + "/" + deref(b.Name), Name: deref(b.Name), TimeCreated: timeOf(b.TimeCreated), Raw: b})
	}

	next := ""
	if resp.OpcNextPage != nil {
		next = *resp.OpcNextPage
	}
	return rows, next, nil
}
