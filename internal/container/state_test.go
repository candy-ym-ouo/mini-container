package container

import (
	"testing"
	"time"
)

func TestStateStoreRoundTrip(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := &Container{ID: "abc", Name: "demo", Image: "busybox:latest", Status: StatusCreated, CreatedAt: time.Now().UTC().Truncate(time.Nanosecond)}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || got.Status != StatusCreated || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("round trip mismatch: %#v", got)
	}
	if err := store.Delete(want.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(want.ID); err != nil {
		t.Fatalf("delete must be idempotent: %v", err)
	}
}

func TestSpecValidation(t *testing.T) {
	valid := Spec{Cmd: []string{"/bin/sh"}, NetworkMode: "bridge", Mounts: []MountSpec{{Source: "/tmp", Target: "/data"}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.NetworkMode = "private"
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid network mode accepted")
	}
}
