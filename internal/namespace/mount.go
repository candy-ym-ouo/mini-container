package namespace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Mount struct {
	Source, Target string
	ReadOnly       bool
}

func PrepareMounts(rootfs string, mounts []Mount) error {
	if runtime.GOOS != "linux" && len(mounts) > 0 {
		return fmt.Errorf("bind mounts require Linux")
	}
	for _, m := range mounts {
		if !filepath.IsAbs(m.Source) || !filepath.IsAbs(m.Target) {
			return fmt.Errorf("mount paths must be absolute")
		}
		info, err := os.Stat(m.Source)
		if err != nil {
			return fmt.Errorf("mount source %s: %w", m.Source, err)
		}
		target := filepath.Join(rootfs, filepath.Clean(m.Target))
		root, _ := filepath.Abs(rootfs)
		clean, _ := filepath.Abs(target)
		if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
			return fmt.Errorf("mount target escapes rootfs")
		}
		if info.IsDir() {
			err = os.MkdirAll(target, 0755)
		} else {
			err = os.MkdirAll(filepath.Dir(target), 0755)
			if err == nil {
				f, e := os.OpenFile(target, os.O_CREATE, 0644)
				if e == nil {
					e = f.Close()
				}
				err = e
			}
		}
		if err != nil {
			return err
		}
		if out, err := exec.Command("mount", "--bind", m.Source, target).CombinedOutput(); err != nil {
			return fmt.Errorf("bind mount: %w: %s", err, strings.TrimSpace(string(out)))
		}
		if m.ReadOnly {
			if out, err := exec.Command("mount", "-o", "remount,bind,ro", target).CombinedOutput(); err != nil {
				return fmt.Errorf("readonly remount: %w: %s", err, strings.TrimSpace(string(out)))
			}
		}
	}
	return nil
}
func PrepareRootfs(rootfs string, mounts []Mount) error {
	if out, err := exec.Command("mount", "--make-rprivate", "/").CombinedOutput(); err != nil {
		return fmt.Errorf("isolate mount propagation: %w: %s", err, strings.TrimSpace(string(out)))
	}
	for _, d := range []string{"proc", "sys", "dev", "dev/pts"} {
		if err := os.MkdirAll(filepath.Join(rootfs, d), 0755); err != nil {
			return err
		}
	}
	commands := [][]string{{"mount", "-t", "proc", "proc", filepath.Join(rootfs, "proc")}, {"mount", "-t", "sysfs", "sysfs", filepath.Join(rootfs, "sys")}, {"mount", "-t", "tmpfs", "-o", "mode=755", "tmpfs", filepath.Join(rootfs, "dev")}}
	for _, a := range commands {
		if out, err := exec.Command(a[0], a[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("prepare rootfs: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	for _, d := range []string{"null", "zero", "random", "urandom", "tty"} {
		target := filepath.Join(rootfs, "dev", d)
		f, err := os.OpenFile(target, os.O_CREATE, 0666)
		if err != nil {
			return err
		}
		_ = f.Close()
		if out, err := exec.Command("mount", "--bind", filepath.Join("/dev", d), target).CombinedOutput(); err != nil {
			return fmt.Errorf("mount /dev/%s: %w: %s", d, err, strings.TrimSpace(string(out)))
		}
	}
	if out, err := exec.Command("mount", "-t", "devpts", "devpts", filepath.Join(rootfs, "dev/pts")).CombinedOutput(); err != nil {
		return fmt.Errorf("mount devpts: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return PrepareMounts(rootfs, mounts)
}
func UnmountAll(rootfs string, mounts []Mount) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	var first error
	for i := len(mounts) - 1; i >= 0; i-- {
		target := filepath.Join(rootfs, filepath.Clean(mounts[i].Target))
		if out, err := exec.Command("umount", target).CombinedOutput(); err != nil && first == nil {
			first = fmt.Errorf("unmount %s: %w: %s", target, err, strings.TrimSpace(string(out)))
		}
	}
	return first
}
