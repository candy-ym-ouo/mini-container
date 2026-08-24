package cgroup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Stats struct {
	CPUUsageNS       uint64
	MemoryBytes      int64
	MemoryLimitBytes int64
	PidsCurrent      int64
	PidsLimit        int64
}

type Manager interface {
	Create() error
	Apply(pid int) error
	Set(Resources) error
	Stats() (Stats, error)
	Remove() error
	Version() int
	Path() string
}

func DetectVersion(root string) (int, error) {
	if _, err := os.Stat(filepath.Join(root, "cgroup.controllers")); err == nil {
		return 2, nil
	}
	if _, err := os.Stat(root); err != nil {
		return 0, fmt.Errorf("inspect cgroup root: %w", err)
	}
	return 1, nil
}

func New(root, id string) (Manager, error) {
	version, err := DetectVersion(root)
	if err != nil {
		return nil, err
	}
	if version == 2 {
		return &v2Manager{path: filepath.Join(root, "mini-container", id)}, nil
	}
	return &v1Manager{root: root, id: id}, nil
}

func writeValue(path, value string) error {
	if err := os.WriteFile(path, []byte(value), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func readValue(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func removeDir(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
