package image

import (
	"archive/tar"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTarRejectsTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.tar")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	data := []byte("bad")
	if err := writer.WriteHeader(&tar.Header{Name: "../outside", Mode: 0644, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	file.Close()
	if err := extractTar(path, t.TempDir()); err == nil {
		t.Fatal("path traversal was accepted")
	}
}

func TestNormalizeName(t *testing.T) {
	name, err := normalizeName("busybox")
	if err != nil || name != "busybox:latest" {
		t.Fatalf("normalize = %q, %v", name, err)
	}
	if _, err := normalizeName("../bad"); err == nil {
		t.Fatal("unsafe name accepted")
	}
}

func TestImportDefaultsTagAndExportPreservesSymlink(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "image.tar")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	w := tar.NewWriter(file)
	manifest, _ := json.Marshal(Manifest{Name: "sample", Layers: []string{"layer0"}})
	entries := []struct {
		name string
		mode int64
		data []byte
		kind byte
		link string
	}{
		{"manifest.json", 0644, manifest, tar.TypeReg, ""},
		{"layer0/", 0755, nil, tar.TypeDir, ""},
		{"layer0/value", 0644, []byte("ok"), tar.TypeReg, ""},
		{"layer0/value-link", 0777, nil, tar.TypeSymlink, "value"}}
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: entry.mode, Size: int64(len(entry.data)), Typeflag: entry.kind, Linkname: entry.link}
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if len(entry.data) > 0 {
			if _, err := w.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(root, "store"), nil)
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Import(archive)
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "sample:latest" {
		t.Fatalf("default name = %s", item.Name)
	}
	exported := filepath.Join(root, "export.tar")
	if err := store.Export(item.Name, exported); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(exported)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	r := tar.NewReader(in)
	found := false
	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == "layer0/value-link" {
			found = true
			if header.Linkname != "value" {
				t.Fatalf("link target = %q", header.Linkname)
			}
		}
	}
	if !found {
		t.Fatal("exported symlink not found")
	}
}
