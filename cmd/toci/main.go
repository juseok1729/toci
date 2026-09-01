package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/spf13/cobra"

	"toci/internal/app"
	"toci/internal/clients"
	"toci/internal/registry"
)

// version is set via -ldflags "-X main.version=..." when building a
// release binary; "dev" otherwise (e.g. `go run`/local builds).
var version = "dev"

func main() {
	var profile, region string
	var write bool

	root := &cobra.Command{
		Use:     "toci",
		Short:   "Terminal UI for Oracle Cloud Infrastructure",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			if profile == "" {
				profile = os.Getenv("OCI_CLI_PROFILE")
			}
			if profile == "" {
				profile = "DEFAULT"
			}

			provider := common.CustomProfileConfigProvider("", profile)
			factory := clients.NewFactory(provider)

			tenancyID, err := factory.TenancyID()
			if err != nil {
				return fmt.Errorf("load tenancy from profile %q: %w", profile, err)
			}

			if region == "" {
				region, err = factory.DefaultRegion()
				if err != nil {
					return fmt.Errorf("load region from profile %q: %w", profile, err)
				}
			}

			scope := registry.Scope{Region: region, CompartmentID: tenancyID}
			m := app.New(factory, scope, write, profile, version)

			_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
			return err
		},
	}

	root.Flags().StringVar(&profile, "profile", "", "OCI config profile (default: $OCI_CLI_PROFILE or DEFAULT)")
	root.Flags().StringVar(&region, "region", "", "override region (default: profile's region)")
	root.Flags().BoolVar(&write, "write", false, "enable write actions (start/stop, ...); default is read-only")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
