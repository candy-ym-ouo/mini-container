package image

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Manifest struct {
	Name   string   `json:"name"`
	Tag    string   `json:"tag"`
	Layers []string `json:"layers"`
}

func extractTar(archive, target string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	r := tar.NewReader(f)
	root, _ := filepath.Abs(target)
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(h.Name)
		path := filepath.Join(root, name)
		if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return fmt.Errorf("archive path escapes target: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, os.FileMode(h.Mode))
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(path), 0755); err == nil {
				var out *os.File
				out, err = os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode))
				if err == nil {
					_, err = io.Copy(out, r)
					e := out.Close()
					if err == nil {
						err = e
					}
				}
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(h.Linkname) || strings.Contains(filepath.Clean(h.Linkname), "..") {
				return fmt.Errorf("unsafe symlink %s", h.Name)
			}
			if err = os.MkdirAll(filepath.Dir(path), 0755); err == nil {
				err = os.Symlink(h.Linkname, path)
			}
		}
		if err != nil {
			return err
		}
	}
}
func (s *Store) Import(archive string) (*Image, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	staging, err := os.MkdirTemp(filepath.Join(s.root, "staging"), "import-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)
	if err = extractTar(archive, staging); err != nil {
		return nil, fmt.Errorf("extract image: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(staging, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var man Manifest
	if err = json.Unmarshal(data, &man); err != nil {
		return nil, err
	}
	input := man.Name
	if man.Tag != "" {
		input += ":" + man.Tag
	}
	name, err := normalizeName(input)
	if err != nil {
		return nil, err
	}
	if _, err = s.registry.Get(name); err == nil {
		return nil, fmt.Errorf("image %s already exists", name)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	size, err := VerifyLayers(staging, man.Layers)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	item := &Image{Name: name, ID: imageID(name, now), SizeBytes: size, Layers: man.Layers, CreatedAt: now}
	dest := filepath.Join(s.root, "layers", item.ID)
	if err = os.MkdirAll(dest, 0755); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dest)
		}
	}()
	for _, layer := range man.Layers {
		if err = CopyTree(filepath.Join(staging, layer), filepath.Join(dest, layer)); err != nil {
			return nil, err
		}
	}
	if err = s.registry.Put(item); err != nil {
		return nil, err
	}
	return item, nil
}
func (s *Store) Export(name, destination string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, err := s.lookup(name)
	if err != nil {
		return err
	}
	f, err := os.Create(destination)
	if err != nil {
		return err
	}
	w := tar.NewWriter(f)
	parts := strings.SplitN(i.Name, ":", 2)
	man := Manifest{Name: parts[0], Tag: parts[1], Layers: i.Layers}
	data, _ := json.MarshalIndent(man, "", "  ")
	if err = w.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0644, Size: int64(len(data))}); err == nil {
		_, err = w.Write(data)
	}
	if err == nil {
		err = addTree(w, filepath.Join(s.root, "layers", i.ID), "")
	}
	if e := w.Close(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	return err
}
func addTree(w *tar.Writer, root, prefix string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(filepath.Join(prefix, rel))
		if err = w.WriteHeader(h); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		e := f.Close()
		if err == nil {
			err = e
		}
		return err
	})
}
