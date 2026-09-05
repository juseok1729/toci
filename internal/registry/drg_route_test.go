package registry

import (
	"context"
	"testing"
)

// Both DRG route resources require Scope.DrgID (the underlying OCI APIs
// take it as a mandatory query param) — without this check, an empty DrgID
// reaches the API as `drgId=` and comes back as a confusing 404
// NotAuthorizedOrNotFound instead of a clear local message.
func TestDrgRouteResourcesRequireDrgID(t *testing.T) {
	ctx := context.Background()
	scope := Scope{Region: "us-phoenix-1", CompartmentID: "ocid1.compartment.oc1..aaa"}

	if _, _, err := (&DrgRouteTableResource{}).List(ctx, scope, ""); err == nil {
		t.Error("DrgRouteTableResource.List() with empty DrgID: got nil error, want one")
	}
	if _, _, err := (&DrgRouteDistributionResource{}).List(ctx, scope, ""); err == nil {
		t.Error("DrgRouteDistributionResource.List() with empty DrgID: got nil error, want one")
	}
}
