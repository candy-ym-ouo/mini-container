package network

import "testing"

func TestIPAMAllocateRelease(t *testing.T) {
	root := t.TempDir()
	ipam, err := NewIPAM(root, "10.0.42.0/24")
	if err != nil {
		t.Fatal(err)
	}
	first, err := ipam.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	second, err := ipam.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if first != "10.0.42.2" || second != "10.0.42.3" {
		t.Fatalf("unexpected addresses %s, %s", first, second)
	}
	if err := ipam.Release(first); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewIPAM(root, "10.0.42.0/24")
	if err != nil {
		t.Fatal(err)
	}
	reused, err := reloaded.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if reused != first {
		t.Fatalf("released address not reused: %s", reused)
	}
}
