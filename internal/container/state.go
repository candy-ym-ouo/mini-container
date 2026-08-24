package container

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type StateStore struct{ dir string }

func NewStateStore(root string) (*StateStore, error) {
	dir := filepath.Join(root, "containers")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &StateStore{dir: dir}, nil
}
func (s *StateStore) path(id string) string { return filepath.Join(s.dir, id+".json") }
func (s *StateStore) Save(c *Container) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".state-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(name, s.path(c.ID))
}
func (s *StateStore) Load(id string) (*Container, error) {
	data, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: container %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	var c Container
	if err = json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
func (s *StateStore) List() ([]*Container, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := []*Container{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		c, err := s.Load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (s *StateStore) Delete(id string) error {
	err := os.Remove(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil || p.Signal(os.Signal(nil)) != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join("/proc", fmt.Sprint(pid), "stat"))
	if err == nil {
		f := strings.Fields(string(data))
		if len(f) > 2 && f[2] == "Z" {
			return false
		}
	}
	return true
}
func (s *StateStore) Recover() ([]*Container, error) {
	items, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, c := range items {
		if (c.Status == StatusRunning || c.Status == StatusStopping) && !ProcessAlive(c.Pid) {
			c.Status = StatusExited
			c.Pid = 0
			if err = s.Save(c); err != nil {
				return nil, err
			}
		}
	}
	return items, nil
}
