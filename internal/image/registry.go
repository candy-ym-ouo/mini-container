package image

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Registry struct{ dir string }

func NewRegistry(dir string) (*Registry, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Registry{dir: dir}, nil
}
func (r *Registry) path(name string) string {
	return filepath.Join(r.dir, strings.ReplaceAll(name, ":", "__")+".json")
}
func (r *Registry) Put(i *Image) error {
	data, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(r.dir, ".image-*")
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
	return os.Rename(name, r.path(i.Name))
}
func (r *Registry) Get(name string) (*Image, error) {
	name, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(r.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return nil, err
	}
	var i Image
	if err = json.Unmarshal(data, &i); err != nil {
		return nil, err
	}
	return &i, nil
}
func (r *Registry) List() ([]*Image, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}
	out := []*Image{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(r.dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var i Image
		if err = json.Unmarshal(data, &i); err != nil {
			return nil, err
		}
		out = append(out, &i)
	}
	return out, nil
}
func (r *Registry) Delete(name string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	if err = os.Remove(r.path(name)); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return err
}
