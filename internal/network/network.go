package network

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Config struct{ Mode, IP, Veth, NSPath string }
type Manager struct {
	ipam           *IPAM
	bridge, subnet string
}

func NewManager(dir, subnet string) (*Manager, error) {
	i, err := NewIPAM(dir, subnet)
	if err != nil {
		return nil, err
	}
	return &Manager{ipam: i, bridge: "minibr0", subnet: subnet}, nil
}
func command(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
func (m *Manager) EnsureBridge() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("bridge networking requires Linux")
	}
	if err := command("ip", "link", "show", m.bridge); err != nil {
		if err := command("ip", "link", "add", m.bridge, "type", "bridge"); err != nil {
			return err
		}
		_ = command("ip", "addr", "add", "10.0.42.1/24", "dev", m.bridge)
	}
	if err := command("ip", "link", "set", m.bridge, "up"); err != nil {
		return err
	}
	_ = command("sysctl", "-w", "net.ipv4.ip_forward=1")
	rule := []string{"-t", "nat", "-C", "POSTROUTING", "-s", m.subnet, "!", "-o", m.bridge, "-j", "MASQUERADE"}
	if err := command("iptables", rule...); err != nil {
		rule[2] = "-A"
		_ = command("iptables", rule...)
	}
	return nil
}
func (m *Manager) Setup(id string, pid int, mode string) (*Config, error) {
	if mode == "host" {
		return &Config{Mode: "host"}, nil
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("container networking requires Linux")
	}
	if err := m.EnsureBridge(); err != nil {
		return nil, err
	}
	ip, err := m.ipam.Allocate()
	if err != nil {
		return nil, err
	}
	host, peer := "vh"+id[:min(8, len(id))], "vc"+id[:min(8, len(id))]
	cleanup := func() { _ = command("ip", "link", "del", host); _ = m.ipam.Release(ip) }
	if err = command("ip", "link", "add", host, "type", "veth", "peer", "name", peer); err != nil {
		cleanup()
		return nil, err
	}
	if err = command("ip", "link", "set", host, "master", m.bridge); err != nil {
		cleanup()
		return nil, err
	}
	if err = command("ip", "link", "set", host, "up"); err != nil {
		cleanup()
		return nil, err
	}
	if err = command("ip", "link", "set", peer, "netns", strconv.Itoa(pid)); err != nil {
		cleanup()
		return nil, err
	}
	ns := []string{"--target", strconv.Itoa(pid), "--net", "ip"}
	if err = command("nsenter", append(ns, "link", "set", peer, "name", "eth0")...); err != nil {
		cleanup()
		return nil, err
	}
	_ = command("nsenter", append(ns, "link", "set", "lo", "up")...)
	if err = command("nsenter", append(ns, "addr", "add", ip+"/24", "dev", "eth0")...); err != nil {
		cleanup()
		return nil, err
	}
	if err = command("nsenter", append(ns, "link", "set", "eth0", "up")...); err != nil {
		cleanup()
		return nil, err
	}
	_ = command("nsenter", append(ns, "route", "add", "default", "via", "10.0.42.1")...)
	return &Config{Mode: "bridge", IP: ip, Veth: host, NSPath: "/proc/" + strconv.Itoa(pid) + "/ns/net"}, nil
}
func (m *Manager) Cleanup(c *Config) error {
	if c == nil || c.Mode == "host" {
		return nil
	}
	var first error
	if runtime.GOOS == "linux" {
		first = command("ip", "link", "del", c.Veth)
	}
	if err := m.ipam.Release(c.IP); err != nil && first == nil {
		first = err
	}
	return first
}
