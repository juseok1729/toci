package app

import (
	"context"

	"toci/internal/registry"
)

// fetchAll walks every page of a List call and returns the combined rows.
// The paging pattern (OpcNextPage -> next request's Page) is identical
// across every OCI list endpoint, so resources don't need to repeat it.
func fetchAll(ctx context.Context, r registry.Resource, s registry.Scope) ([]registry.Row, error) {
	var all []registry.Row
	page := ""
	for {
		rows, next, err := r.List(ctx, s, page)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
		if next == "" {
			return all, nil
		}
		page = next
	}
}
