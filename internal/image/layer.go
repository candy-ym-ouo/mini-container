package image

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func LayerDigest(root string) (string, int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			size += info.Size()
		}
		return nil
	})
	return "", size, err
}

func VerifyLayers(root string, layers []string) (int64, error) {
	if len(layers) == 0 {
		return 0, fmt.Errorf("manifest contains no layers")
	}
	seen := map[string]bool{}
	var total int64
	for _, layer := range layers {
		if layer == "" || layer == "." || layer == ".." || filepath.Base(layer) != layer {
			return 0, fmt.Errorf("invalid layer %q", layer)
		}
		if seen[layer] {
			return 0, fmt.Errorf("duplicate layer %q", layer)
		}
		seen[layer] = true
		path := filepath.Join(root, layer)
		info, err := os.Stat(path)
		if err != nil {
			return 0, fmt.Errorf("layer %s: %w", layer, err)
		}
		if !info.IsDir() {
			return 0, fmt.Errorf("layer %s is not a directory", layer)
		}
		_, size, err := LayerDigest(path)
		if err != nil {
			return 0, err
		}
		total += size
	}
	return total, nil
}

func CopyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(dst, info.Mode().Perm())
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, dst)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		inCloseErr := in.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if inCloseErr != nil {
			return inCloseErr
		}
		return closeErr
	})
}
