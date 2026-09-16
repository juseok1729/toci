package registry

import "testing"

func TestGatewayResourceColumnsReadGatewayRow(t *testing.T) {
	r := NewGatewayResource(nil)
	row := Row{Raw: gatewayRow{Type: "NAT", Name: "my-natgw", State: "Available", Detail: "10.0.0.5"}}

	want := map[string]string{
		"NAME":   "my-natgw",
		"TYPE":   "NAT",
		"STATE":  "Available",
		"DETAIL": "10.0.0.5",
	}
	for _, col := range r.Columns() {
		if want[col.Header] != col.Get(row) {
			t.Errorf("column %q = %q, want %q", col.Header, col.Get(row), want[col.Header])
		}
	}
}

func TestYesNo(t *testing.T) {
	yes := true
	no := false
	if got := yesNo(&yes); got != "Yes" {
		t.Errorf("yesNo(true) = %q, want Yes", got)
	}
	if got := yesNo(&no); got != "No" {
		t.Errorf("yesNo(false) = %q, want No", got)
	}
	if got := yesNo(nil); got != "No" {
		t.Errorf("yesNo(nil) = %q, want No", got)
	}
}
