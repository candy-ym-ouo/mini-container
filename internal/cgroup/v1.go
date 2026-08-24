package cgroup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type v1Manager struct{ root, id string }

func (m *v1Manager) controller(name string) string {
	candidates := []string{name}
	if name == "cpu" || name == "cpuacct" {
		candidates = []string{"cpu,cpuacct", "cpuacct,cpu", name}
	}
	for _, c := range candidates {
		if info, err := os.Stat(filepath.Join(m.root, c)); err == nil && info.IsDir() {
			return filepath.Join(m.root, c, "mini-container", m.id)
		}
	}
	return filepath.Join(m.root, name, "mini-container", m.id)
}
func (m *v1Manager) Version() int { return 1 }
func (m *v1Manager) Path() string { return m.controller("cpu") }
func (m *v1Manager) Create() error {
	for _, n := range []string{"cpu", "memory", "pids"} {
		if err := os.MkdirAll(m.controller(n), 0755); err != nil {
			return err
		}
	}
	return nil
}
func (m *v1Manager) Apply(pid int) error {
	for _, n := range []string{"cpu", "memory", "pids"} {
		if err := writeValue(filepath.Join(m.controller(n), "tasks"), strconv.Itoa(pid)); err != nil {
			return err
		}
	}
	return nil
}
func (m *v1Manager) Set(r Resources) error {
	if err := Validate(r); err != nil {
		return err
	}
	if r.CPUShares > 0 {
		if err := writeValue(filepath.Join(m.controller("cpu"), "cpu.shares"), strconv.FormatInt(r.CPUShares, 10)); err != nil {
			return err
		}
	}
	if r.CPUQuota > 0 {
		q, p := CPUQuota(r.CPUQuota)
		if err := writeValue(filepath.Join(m.controller("cpu"), "cpu.cfs_period_us"), p); err != nil {
			return err
		}
		if err := writeValue(filepath.Join(m.controller("cpu"), "cpu.cfs_quota_us"), q); err != nil {
			return err
		}
	}
	if r.MemoryMB > 0 {
		limit := strconv.FormatInt(r.MemoryMB*1024*1024, 10)
		if err := writeValue(filepath.Join(m.controller("memory"), "memory.limit_in_bytes"), limit); err != nil {
			return err
		}
		_ = writeValue(filepath.Join(m.controller("memory"), "memory.swappiness"), "0")
	}
	if r.PidsLimit > 0 {
		return writeValue(filepath.Join(m.controller("pids"), "pids.max"), strconv.FormatInt(r.PidsLimit, 10))
	}
	return nil
}
func (m *v1Manager) Stats() (Stats, error) {
	var s Stats
	read := func(c, f string) (int64, error) {
		v, e := readValue(filepath.Join(m.controller(c), f))
		if e != nil {
			return 0, e
		}
		v = strings.TrimSpace(v)
		if v == "max" {
			return -1, nil
		}
		return strconv.ParseInt(v, 10, 64)
	}
	cpu, e := read("cpuacct", "cpuacct.usage")
	if e == nil {
		s.CPUUsageNS = uint64(cpu)
	}
	if s.MemoryBytes, e = read("memory", "memory.usage_in_bytes"); e != nil {
		return s, fmt.Errorf("read memory: %w", e)
	}
	s.MemoryLimitBytes, _ = read("memory", "memory.limit_in_bytes")
	s.PidsCurrent, _ = read("pids", "pids.current")
	s.PidsLimit, _ = read("pids", "pids.max")
	return s, nil
}
func (m *v1Manager) Remove() error {
	var first error
	for _, n := range []string{"pids", "memory", "cpu"} {
		if err := removeDir(m.controller(n)); err != nil && first == nil {
			first = err
		}
	}
	return first
}
