package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type IPAM struct {
	mu     sync.Mutex
	path   string
	subnet *net.IPNet
	used   map[string]bool
}

func NewIPAM(dir, cidr string) (*IPAM, error) {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse subnet: %w", err)
	}
	i := &IPAM{path: filepath.Join(dir, "ipam.json"), subnet: subnet, used: map[string]bool{}}
	data, err := os.ReadFile(i.path)
	if err == nil {
		if err = json.Unmarshal(data, &i.used); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return i, nil
}
func (i *IPAM) save() error {
	data, err := json.MarshalIndent(i.used, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(i.path), 0755); err != nil {
		return err
	}
	return os.WriteFile(i.path, data, 0644)
}
func (i *IPAM) Allocate() (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	b := i.subnet.IP.To4()
	if b == nil {
		return "", fmt.Errorf("only IPv4 subnets are supported")
	}
	for h := 2; h < 255; h++ {
		ip := net.IPv4(b[0], b[1], b[2], byte(h)).String()
		if !i.subnet.Contains(net.ParseIP(ip)) {
			break
		}
		if i.used[ip] {
			continue
		}
		i.used[ip] = true
		if err := i.save(); err != nil {
			delete(i.used, ip)
			return "", err
		}
		return ip, nil
	}
	return "", fmt.Errorf("subnet %s is exhausted", i.subnet)
}
func (i *IPAM) Release(ip string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.used, ip)
	return i.save()
}
func (i *IPAM) Allocations() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]string, 0, len(i.used))
	for ip := range i.used {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}
