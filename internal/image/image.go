package image

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("image not found")
var ErrReferenced = errors.New("image is referenced")

type Image struct {
	Name      string    `json:"name"`
	ID        string    `json:"id"`
	SizeBytes int64     `json:"sizeBytes"`
	Layers    []string  `json:"layers"`
	CreatedAt time.Time `json:"createdAt"`
	RefCount  int       `json:"refCount"`
}
type Store struct {
	mu       sync.RWMutex
	root     string
	registry *Registry
	refs     func(string) int
}

func NewStore(root string, refs func(string) int) (*Store, error) {
	for _, d := range []string{"images", "layers", "staging"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			return nil, err
		}
	}
	r, err := NewRegistry(filepath.Join(root, "images"))
	if err != nil {
		return nil, err
	}
	if refs == nil {
		refs = func(string) int { return 0 }
	}
	return &Store{root: root, registry: r, refs: refs}, nil
}
func (s *Store) Root() string { return s.root }
func (s *Store) lookup(name string) (*Image, error) {
	i, err := s.registry.Get(name)
	if err != nil {
		return nil, err
	}
	i.RefCount = s.refs(i.Name)
	return i, nil
}
func (s *Store) Lookup(name string) (*Image, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lookup(name)
}
func (s *Store) List() ([]*Image, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items, err := s.registry.List()
	if err != nil {
		return nil, err
	}
	for _, i := range items {
		i.RefCount = s.refs(i.Name)
	}
	sort.Slice(items, func(a, b int) bool { return items[a].Name < items[b].Name })
	return items, nil
}
func (s *Store) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.lookup(name)
	if err != nil {
		return err
	}
	if i.RefCount > 0 {
		return fmt.Errorf("%w: %s has %d container references", ErrReferenced, name, i.RefCount)
	}
	if err = s.registry.Delete(name); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.root, "layers", i.ID))
}
func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("image name is required")
	}
	if !strings.Contains(name, ":") {
		name += ":latest"
	}
	if strings.ContainsAny(name, `/\\`) || strings.Contains(name, "..") || strings.ContainsAny(name, "\r\n\t\"'") || strings.Count(name, ":") != 1 || strings.ContainsAny(name, " \t") {
		return "", fmt.Errorf("invalid image name %q", name)
	}
	p := strings.SplitN(name, ":", 2)
	if p[0] == "" || p[1] == "" {
		return "", fmt.Errorf("invalid image name %q", name)
	}
	return name, nil
}
func imageID(name string, t time.Time) string {
	sum := sha256.Sum256([]byte(name + t.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(sum[:8])
}
