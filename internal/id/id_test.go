package id

import "testing"

func TestNewEndpointID(t *testing.T) {
	id := NewEndpointID()
	if len(id) != EndpointIDLength {
		t.Errorf("expected length %d, got %d", EndpointIDLength, len(id))
	}
}

func TestNewEndpointIDUnique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewEndpointID()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}
