package registry

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func TestNsgResourceColumnsReadNsgRow(t *testing.T) {
	r := NewNsgResource(nil)
	row := Row{Raw: NsgRow{
		NetworkSecurityGroup: core.NetworkSecurityGroup{DisplayName: common.String("my-nsg")},
		RuleCount:            5,
	}}

	want := map[string]string{
		"NAME":  "my-nsg",
		"RULES": "5",
	}
	for _, col := range r.Columns() {
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}
