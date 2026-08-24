package cgroup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2WeightAndQuota(t *testing.T) {
	if got := V2Weight(2); got != 1 {
		t.Fatalf("minimum weight = %d", got)
	}
	if got := V2Weight(262144); got != 10000 {
		t.Fatalf("maximum weight = %d", got)
	}
	if got := V2Weight(1024); got < 39 || got > 40 {
		t.Fatalf("default weight = %d", got)
	}
	if got := CPUQuotaV2(1.5); got != "150000 100000" {
		t.Fatalf("quota = %q", got)
	}
	if err := Validate(Resources{MemoryMB: -1}); err == nil {
		t.Fatal("negative memory accepted")
	}
}

func TestV2CreateEnablesDelegatedControllers(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "mini-container")
	if err := os.MkdirAll(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "cgroup.controllers"), []byte("cpu io memory pids"), 0644); err != nil {
		t.Fatal(err)
	}
	manager := &v2Manager{path: filepath.Join(parent, "demo")}
	if err := manager.Create(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(parent, "cgroup.subtree_control"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"+cpu", "+memory", "+pids"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("subtree control %q lacks %s", data, want)
		}
	}
}
