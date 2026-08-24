package cgroup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type v2Manager struct{ path string }

func (m *v2Manager) Version() int { return 2 }
func (m *v2Manager) Path() string { return m.path }
func (m *v2Manager) Create() error {
	parent := filepath.Dir(m.path)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(parent, "cgroup.controllers"))
	if err != nil {
		return fmt.Errorf("read delegated controllers: %w", err)
	}
	have := map[string]bool{}
	for _, n := range strings.Fields(string(data)) {
		have[n] = true
	}
	var req []string
	for _, n := range []string{"cpu", "memory", "pids"} {
		if have[n] {
			req = append(req, "+"+n)
		}
	}
	if len(req) > 0 {
		if err := writeValue(filepath.Join(parent, "cgroup.subtree_control"), strings.Join(req, " ")); err != nil {
			return err
		}
	}
	return os.MkdirAll(m.path, 0755)
}
func (m *v2Manager) Apply(pid int) error {
	return writeValue(filepath.Join(m.path, "cgroup.procs"), strconv.Itoa(pid))
}
func (m *v2Manager) Set(r Resources) error {
	if err := Validate(r); err != nil {
		return err
	}
	if r.CPUShares > 0 {
		if err := writeValue(filepath.Join(m.path, "cpu.weight"), strconv.FormatInt(V2Weight(r.CPUShares), 10)); err != nil {
			return err
		}
	}
	if r.CPUQuota > 0 {
		if err := writeValue(filepath.Join(m.path, "cpu.max"), CPUQuotaV2(r.CPUQuota)); err != nil {
			return err
		}
	}
	if r.MemoryMB > 0 {
		limit := strconv.FormatInt(r.MemoryMB*1024*1024, 10)
		_ = writeValue(filepath.Join(m.path, "memory.swap.max"), "0")
		if err := writeValue(filepath.Join(m.path, "memory.max"), limit); err != nil {
			return err
		}
	}
	if r.PidsLimit > 0 {
		return writeValue(filepath.Join(m.path, "pids.max"), strconv.FormatInt(r.PidsLimit, 10))
	}
	return nil
}
func parseLimit(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "max" {
		return -1, nil
	}
	return strconv.ParseInt(v, 10, 64)
}
func (m *v2Manager) Stats() (Stats, error) {
	var s Stats
	file, _ := os.Open(filepath.Join(m.path, "cpu.stat"))
	if file != nil {
		scan := bufio.NewScanner(file)
		for scan.Scan() {
			f := strings.Fields(scan.Text())
			if len(f) == 2 && f[0] == "usage_usec" {
				u, _ := strconv.ParseUint(f[1], 10, 64)
				s.CPUUsageNS = u * 1000
			}
		}
		_ = file.Close()
	}
	read := func(n string) (int64, error) {
		v, e := readValue(filepath.Join(m.path, n))
		if e != nil {
			return 0, e
		}
		return parseLimit(v)
	}
	var err error
	if s.MemoryBytes, err = read("memory.current"); err != nil {
		return s, fmt.Errorf("read memory.current: %w", err)
	}
	s.MemoryLimitBytes, _ = read("memory.max")
	s.PidsCurrent, _ = read("pids.current")
	s.PidsLimit, _ = read("pids.max")
	return s, nil
}
func (m *v2Manager) Remove() error { return removeDir(m.path) }
