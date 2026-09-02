// Package clients builds and caches OCI SDK service clients per region.
package clients

import (
	"fmt"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/loadbalancer"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

// Factory hands out SDK clients bound to a region, caching one instance per
// (region, client type) pair so region switches don't leak old clients.
type Factory struct {
	provider common.ConfigurationProvider

	mu    sync.Mutex
	cache map[string]any // "region:type" -> client
}

func NewFactory(provider common.ConfigurationProvider) *Factory {
	return &Factory{provider: provider, cache: make(map[string]any)}
}

// TenancyID returns the tenancy OCID from the underlying profile.
func (f *Factory) TenancyID() (string, error) {
	return f.provider.TenancyOCID()
}

// DefaultRegion returns the region configured in the active profile.
func (f *Factory) DefaultRegion() (string, error) {
	return f.provider.Region()
}

func get[T any](f *Factory, region, kind string, build func() (T, error)) (T, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := region + ":" + kind
	if c, ok := f.cache[key]; ok {
		return c.(T), nil
	}
	client, err := build()
	if err != nil {
		var zero T
		return zero, fmt.Errorf("build %s client: %w", kind, err)
	}
	f.cache[key] = client
	return client, nil
}

func (f *Factory) Identity(region string) (identity.IdentityClient, error) {
	return get(f, region, "identity", func() (identity.IdentityClient, error) {
		c, err := identity.NewIdentityClientWithConfigurationProvider(f.provider)
		if err != nil {
			return identity.IdentityClient{}, err
		}
		c.SetRegion(region)
		return c, nil
	})
}

func (f *Factory) VirtualNetwork(region string) (core.VirtualNetworkClient, error) {
	return get(f, region, "vcn", func() (core.VirtualNetworkClient, error) {
		c, err := core.NewVirtualNetworkClientWithConfigurationProvider(f.provider)
		if err != nil {
			return core.VirtualNetworkClient{}, err
		}
		c.SetRegion(region)
		return c, nil
	})
}

func (f *Factory) Compute(region string) (core.ComputeClient, error) {
	return get(f, region, "compute", func() (core.ComputeClient, error) {
		c, err := core.NewComputeClientWithConfigurationProvider(f.provider)
		if err != nil {
			return core.ComputeClient{}, err
		}
		c.SetRegion(region)
		return c, nil
	})
}

func (f *Factory) Blockstorage(region string) (core.BlockstorageClient, error) {
	return get(f, region, "blockstorage", func() (core.BlockstorageClient, error) {
		c, err := core.NewBlockstorageClientWithConfigurationProvider(f.provider)
		if err != nil {
			return core.BlockstorageClient{}, err
		}
		c.SetRegion(region)
		return c, nil
	})
}

func (f *Factory) Bastion(region string) (bastion.BastionClient, error) {
	return get(f, region, "bastion", func() (bastion.BastionClient, error) {
		c, err := bastion.NewBastionClientWithConfigurationProvider(f.provider)
		if err != nil {
			return bastion.BastionClient{}, err
		}
		c.SetRegion(region)
		return c, nil
	})
}

func (f *Factory) Monitoring(region string) (monitoring.MonitoringClient, error) {
	return get(f, region, "monitoring", func() (monitoring.MonitoringClient, error) {
		c, err := monitoring.NewMonitoringClientWithConfigurationProvider(f.provider)
		if err != nil {
			return monitoring.MonitoringClient{}, err
		}
		c.SetRegion(region)
		return c, nil
	})
}

func (f *Factory) Database(region string) (database.DatabaseClient, error) {
	return get(f, region, "database", func() (database.DatabaseClient, error) {
		c, err := database.NewDatabaseClientWithConfigurationProvider(f.provider)
		if err != nil {
			return database.DatabaseClient{}, err
		}
		c.SetRegion(region)
		return c, nil
	})
}

func (f *Factory) LoadBalancer(region string) (loadbalancer.LoadBalancerClient, error) {
	return get(f, region, "lb", func() (loadbalancer.LoadBalancerClient, error) {
		c, err := loadbalancer.NewLoadBalancerClientWithConfigurationProvider(f.provider)
		if err != nil {
			return loadbalancer.LoadBalancerClient{}, err
		}
		c.SetRegion(region)
		return c, nil
	})
}
