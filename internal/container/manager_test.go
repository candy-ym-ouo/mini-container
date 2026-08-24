package container

import (
	"context"
	"testing"
)

func TestWaitCreatedContainerReturnsConflict(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager.items["created"] = &Container{ID: "created", Name: "created", Status: StatusCreated}
	if _, err := manager.Wait(context.Background(), "created"); err == nil {
		t.Fatal("wait accepted a created container")
	}
}
