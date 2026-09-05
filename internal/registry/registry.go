package registry

import "toci/internal/clients"

// All returns every resource kind the UI can browse, in display order.
func All(f *clients.Factory) []Resource {
	return []Resource{
		NewCompartmentResource(f),
		NewInstanceResource(f),
		NewVcnResource(f),
		NewSubnetResource(f),
		NewRouteTableResource(f),
		NewSecurityListResource(f),
		NewNsgResource(f),
		NewDrgResource(f),
		NewDrgAttachmentResource(f),
		NewDrgRouteTableResource(f),
		NewDrgRouteDistributionResource(f),
		NewLoadBalancerResource(f),
		NewDbSystemResource(f),
		NewAutonomousDatabaseResource(f),
		NewCloudVmClusterResource(f),
	}
}
