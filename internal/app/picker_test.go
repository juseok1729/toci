package app

import "testing"

// items models resourcePickerItems' shape: a category header (key == "")
// followed by its resources.
var pickerTestItems = []pickerItem{
	{label: "Compute"},
	{key: "instance", label: "Instances"},
	{label: "Network"},
	{key: "vcn", label: "VCNs"},
	{key: "subnet", label: "Subnets"},
}

func TestPickerCursorUpDownSkipsCategoryHeaders(t *testing.T) {
	p := picker{filtered: pickerTestItems, cursor: 1} // on "Instances"

	p.cursorDown() // should skip the "Network" header and land on "VCNs"
	if got := p.filtered[p.cursor].key; got != "vcn" {
		t.Errorf("cursorDown from Instances landed on %q, want vcn", got)
	}

	p.cursorDown() // Subnets, no header in between
	if got := p.filtered[p.cursor].key; got != "subnet" {
		t.Errorf("cursorDown from VCNs landed on %q, want subnet", got)
	}

	p.cursorDown() // already at the last item — no-op
	if got := p.filtered[p.cursor].key; got != "subnet" {
		t.Errorf("cursorDown past the last item landed on %q, want subnet (no-op)", got)
	}

	p.cursorUp() // back to VCNs, skipping the Network header
	if got := p.filtered[p.cursor].key; got != "vcn" {
		t.Errorf("cursorUp from Subnets landed on %q, want vcn", got)
	}

	p.cursorUp() // Instances, skipping the Compute header
	if got := p.filtered[p.cursor].key; got != "instance" {
		t.Errorf("cursorUp from VCNs landed on %q, want instance", got)
	}

	p.cursorUp() // already at the first selectable item — no-op
	if got := p.filtered[p.cursor].key; got != "instance" {
		t.Errorf("cursorUp past the first item landed on %q, want instance (no-op)", got)
	}
}

func TestPickerRefilterNeverLeavesCursorOnACategoryHeader(t *testing.T) {
	p := picker{items: pickerTestItems, filtered: pickerTestItems}
	p.refilter() // empty query — refilter() still clamps/fixes up the cursor

	if got := p.filtered[p.cursor].key; got == "" {
		t.Errorf("refilter() left the cursor on a category header (index %d, label %q)", p.cursor, p.filtered[p.cursor].label)
	}
}
