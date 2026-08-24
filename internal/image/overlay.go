package image

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Mount struct {
	Device, Target string
	ReadOnly       bool
}

func (s *Store) MountRootfs(name, containerRoot string) ([]Mount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	rootfs, upper, work := filepath.Join(containerRoot, "rootfs"), filepath.Join(containerRoot, "upper"), filepath.Join(containerRoot, "work")
	for _, d := range []string{rootfs, upper, work} {
		if err = os.MkdirAll(d, 0755); err != nil {
			return nil, err
		}
	}
	lower := []string{}
	for n := len(i.Layers) - 1; n >= 0; n-- {
		lower = append(lower, filepath.Join(s.root, "layers", i.ID, i.Layers[n]))
	}
	if runtime.GOOS != "linux" {
		return []Mount{{Device: "overlay-simulated", Target: rootfs}}, nil
	}
	opts := "lowerdir=" + strings.Join(lower, ":") + ",upperdir=" + upper + ",workdir=" + work
	if len(opts) > 3500 {
		return s.copyFallback(lower, rootfs)
	}
	out, err := exec.Command("mount", "-t", "overlay", "overlay", "-o", opts, rootfs).CombinedOutput()
	if err != nil {
		// Rootless test/build containers commonly lack CAP_SYS_ADMIN. In that
		// case the already validated copy mode preserves management semantics;
		// retain hard failures for malformed layers and other mount errors.
		message := strings.ToLower(string(out))
		if strings.Contains(message, "permission denied") || strings.Contains(message, "operation not permitted") {
			return s.copyFallback(lower, rootfs)
		}
		return nil, fmt.Errorf("mount overlay: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return []Mount{{Device: "overlay", Target: rootfs}}, nil
}
func (s *Store) copyFallback(layers []string, rootfs string) ([]Mount, error) {
	for n := len(layers) - 1; n >= 0; n-- {
		if err := CopyTree(layers[n], rootfs); err != nil {
			return nil, err
		}
	}
	return []Mount{{Device: "copy-fallback", Target: rootfs}}, nil
}
func (s *Store) UnmountRootfs(m []Mount) error {
	var first error
	for n := len(m) - 1; n >= 0; n-- {
		if m[n].Device != "overlay" || runtime.GOOS != "linux" {
			continue
		}
		out, err := exec.Command("umount", m[n].Target).CombinedOutput()
		if err != nil && first == nil {
			first = fmt.Errorf("unmount %s: %w: %s", m[n].Target, err, strings.TrimSpace(string(out)))
		}
	}
	return first
}
