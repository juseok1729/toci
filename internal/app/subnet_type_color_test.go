package app

import (
	"reflect"
	"testing"
)

func TestSubnetTypeStyleFor(t *testing.T) {
	if got, ok := subnetTypeStyleFor("Public", false); !ok || !reflect.DeepEqual(got, stateTextGood) {
		t.Errorf("subnetTypeStyleFor(Public, false) = %v, %v, want stateTextGood, true", got, ok)
	}
	if got, ok := subnetTypeStyleFor("Public", true); !ok || !reflect.DeepEqual(got, stateTextGoodSelected) {
		t.Errorf("subnetTypeStyleFor(Public, true) = %v, %v, want stateTextGoodSelected, true", got, ok)
	}
	if got, ok := subnetTypeStyleFor("Private", false); !ok || !reflect.DeepEqual(got, subnetTypeColorPrivate) {
		t.Errorf("subnetTypeStyleFor(Private, false) = %v, %v, want subnetTypeColorPrivate, true", got, ok)
	}
	if got, ok := subnetTypeStyleFor("Private", true); !ok || !reflect.DeepEqual(got, subnetTypeColorPrivateSelected) {
		t.Errorf("subnetTypeStyleFor(Private, true) = %v, %v, want subnetTypeColorPrivateSelected, true", got, ok)
	}

	// A DRG Attachment/DRG Route Distribution's own TYPE value (e.g. "VCN")
	// must not match — colorizeSubnetType applies to any table with a
	// "TYPE" column, not just Subnets.
	if _, ok := subnetTypeStyleFor("VCN", false); ok {
		t.Error("subnetTypeStyleFor matched an unrelated TYPE value, want no match")
	}
}
