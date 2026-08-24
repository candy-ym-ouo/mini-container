package container

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"mini-container/internal/cgroup"
	execpkg "mini-container/internal/exec"
	"mini-container/internal/image"
	"mini-container/internal/network"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Event struct {
	Type     string `json:"type"`
	Action   string `json:"action"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
}
type Manager struct {
	mu             sync.RWMutex
	operations     sync.Mutex
	root           string
	states         *StateStore
	images         *image.Store
	network        *network.Manager
	items          map[string]*Container
	processes      map[string]*Process
	creatingImages map[string]int
	statsSamples   map[string]cpuSample
	subscribers    map[chan Event]struct{}
}
type cpuSample struct {
	usage uint64
	at    time.Time
}

func NewManager(root string) (*Manager, error) {
	if root == "" {
		return nil, fmt.Errorf("state directory is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	for _, d := range []string{"containers", "logs", "images", "layers"} {
		if err = os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			return nil, err
		}
	}
	states, err := NewStateStore(root)
	if err != nil {
		return nil, err
	}
	m := &Manager{root: root, states: states, items: map[string]*Container{}, processes: map[string]*Process{}, creatingImages: map[string]int{}, statsSamples: map[string]cpuSample{}, subscribers: map[chan Event]struct{}{}}
	m.images, err = image.NewStore(root, m.ImageReferences)
	if err != nil {
		return nil, err
	}
	m.network, err = network.NewManager(root, "10.0.42.0/24")
	if err != nil {
		return nil, err
	}
	items, err := states.Recover()
	if err != nil {
		return nil, err
	}
	for _, i := range items {
		m.items[i.ID] = i
	}
	return m, nil
}
func (m *Manager) Root() string         { return m.root }
func (m *Manager) Images() *image.Store { return m.images }
func generateID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (m *Manager) resolveLocked(id string) (*Container, error) {
	if c, ok := m.items[id]; ok {
		return c, nil
	}
	for _, c := range m.items {
		if c.Name == id {
			return c, nil
		}
	}
	return nil, fmt.Errorf("%w: container %s", ErrNotFound, id)
}
func cloneContainer(c *Container) *Container {
	x := *c
	x.Spec.Cmd = c.Spec.Cmd
	x.Spec.Env = c.Spec.Env
	x.Spec.Mounts = c.Spec.Mounts
	x.Mounts = c.Mounts
	if c.Net != nil {
		v := *c.Net
		x.Net = &v
	}
	if c.Cgroup != nil {
		v := *c.Cgroup
		x.Cgroup = &v
	}
	return &x
}
func (m *Manager) Create(o CreateOptions) (*Container, error) {
	m.operations.Lock()
	defer m.operations.Unlock()
	if strings.TrimSpace(o.Image) == "" {
		return nil, fmt.Errorf("%w: image is required", ErrInvalidParam)
	}
	if err := o.Spec.Validate(); err != nil {
		return nil, err
	}
	img, err := m.images.Lookup(o.Image)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrImageMissing, err)
	}
	o.Image = img.Name
	m.mu.Lock()
	m.creatingImages[o.Image]++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.creatingImages[o.Image]--
		if m.creatingImages[o.Image] == 0 {
			delete(m.creatingImages, o.Image)
		}
		m.mu.Unlock()
	}()
	id, err := generateID()
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(o.Name)
	if name == "" {
		name = "mc-" + id[:8]
	}
	if o.Spec.Hostname == "" {
		o.Spec.Hostname = name
	}
	if o.Spec.NetworkMode == "" {
		o.Spec.NetworkMode = "bridge"
	}
	if o.Spec.Resources.CPUShares == 0 {
		o.Spec.Resources.CPUShares = 1024
	}
	root := filepath.Join(m.root, "containers", id)
	m.mu.Lock()
	for _, c := range m.items {
		if c.Name == name {
			m.mu.Unlock()
			return nil, fmt.Errorf("%w: container name %s already exists", ErrConflict, name)
		}
	}
	m.mu.Unlock()
	mounts, err := m.images.MountRootfs(o.Image, root)
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}
	records := make([]MountRecord, len(mounts))
	for i, x := range mounts {
		records[i] = MountRecord{Device: x.Device, Target: x.Target, ReadOnly: x.ReadOnly}
	}
	c := &Container{ID: id, Name: name, Image: o.Image, Spec: o.Spec, Status: StatusCreated, Mounts: records, LogFile: filepath.Join(m.root, "logs", id+".log"), CreatedAt: time.Now().UTC(), AutoRemove: o.AutoRemove}
	if err = os.WriteFile(c.LogFile, nil, 0644); err != nil {
		_ = m.images.UnmountRootfs(mounts)
		_ = os.RemoveAll(root)
		return nil, err
	}
	if err = m.states.Save(c); err != nil {
		_ = m.images.UnmountRootfs(mounts)
		_ = os.RemoveAll(root)
		return nil, err
	}
	m.mu.Lock()
	m.items[id] = c
	m.mu.Unlock()
	m.publish(Event{Type: "container", Action: "created", ID: id, Name: name})
	return cloneContainer(c), nil
}
func (m *Manager) Start(id string) (*Container, error) {
	m.mu.Lock()
	c, err := m.resolveLocked(id)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if c.Status != StatusCreated && c.Status != StatusExited {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: cannot start container in %s state", ErrConflict, c.Status)
	}
	root := filepath.Join(m.root, "containers", c.ID)
	p, err := StartProcess(c, root)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	r := cgroup.Resources{CPUShares: c.Spec.Resources.CPUShares, CPUQuota: c.Spec.Resources.CPUQuota, MemoryMB: c.Spec.Resources.MemoryMB, PidsLimit: c.Spec.Resources.PidsLimit}
	cg, err := cgroup.New("/sys/fs/cgroup", c.ID)
	if err == nil {
		err = cg.Create()
	}
	if err == nil {
		err = cg.Set(r)
	}
	if err == nil {
		err = cg.Apply(p.PID)
	}
	if err != nil {
		_ = p.Kill()
		if cg != nil {
			_ = cg.Remove()
		}
		m.mu.Unlock()
		return nil, fmt.Errorf("configure cgroup: %w", err)
	}
	netc, err := m.network.Setup(c.ID, p.PID, c.Spec.NetworkMode)
	if err != nil {
		_ = p.Kill()
		_ = cg.Remove()
		m.mu.Unlock()
		return nil, fmt.Errorf("configure network: %w", err)
	}
	c.Pid, c.Status, c.StartedAt = p.PID, StatusRunning, time.Now().UTC()
	c.Net = &NetConfig{Mode: netc.Mode, IP: netc.IP, Veth: netc.Veth, NSPath: netc.NSPath}
	c.Cgroup = &CgroupConfig{Version: cg.Version(), Path: cg.Path()}
	m.processes[c.ID] = p
	delete(m.statsSamples, c.ID)
	if err = m.states.Save(c); err != nil {
		_ = p.Kill()
		_ = m.network.Cleanup(netc)
		_ = cg.Remove()
		c.Pid = 0
		c.Status = StatusCreated
		c.Net = nil
		c.Cgroup = nil
		delete(m.processes, c.ID)
		m.mu.Unlock()
		return nil, err
	}
	if err = p.Release(); err != nil {
		_ = p.Kill()
		_ = m.network.Cleanup(netc)
		_ = cg.Remove()
		c.Pid = 0
		c.Status = StatusCreated
		c.Net = nil
		c.Cgroup = nil
		delete(m.processes, c.ID)
		_ = m.states.Save(c)
		m.mu.Unlock()
		return nil, err
	}
	out := cloneContainer(c)
	m.mu.Unlock()
	m.publish(Event{Type: "container", Action: "started", ID: out.ID, Name: out.Name})
	go m.monitor(out.ID, p, cg)
	return out, nil
}
func (m *Manager) monitor(id string, p *Process, cg cgroup.Manager) {
	<-p.Done()
	result := p.Result()
	m.mu.Lock()
	c, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	c.Status = StatusExited
	c.Pid = 0
	c.ExitCode = result.code
	c.ExitedAt = time.Now().UTC()
	delete(m.processes, id)
	delete(m.statsSamples, id)
	_ = m.states.Save(c)
	auto, name := c.AutoRemove, c.Name
	m.mu.Unlock()
	_ = cg.Remove()
	m.publish(Event{Type: "container", Action: "exited", ID: id, Name: name, ExitCode: result.code})
	if auto {
		_ = m.Remove(id)
	}
}
func (m *Manager) Stop(id string, timeout time.Duration) (*Container, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	m.mu.Lock()
	c, err := m.resolveLocked(id)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if c.Status == StatusCreated || c.Status == StatusExited {
		out := cloneContainer(c)
		m.mu.Unlock()
		return out, nil
	}
	if c.Status != StatusRunning && c.Status != StatusStopping {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: cannot stop %s", ErrConflict, c.Status)
	}
	c.Status = StatusStopping
	if err = m.states.Save(c); err != nil {
		c.Status = StatusRunning
		m.mu.Unlock()
		return nil, err
	}
	p := m.processes[c.ID]
	pid := c.Pid
	m.mu.Unlock()
	if p != nil {
		_ = p.Terminate()
		select {
		case <-p.Done():
		case <-time.After(timeout):
			_ = p.Kill()
		}
	} else if pid > 0 {
		if proc, e := os.FindProcess(pid); e == nil {
			_ = proc.Signal(syscall.SIGTERM)
		}
		m.mu.Lock()
		if current, ok := m.items[c.ID]; ok {
			current.Status, current.Pid, current.ExitCode, current.ExitedAt = StatusExited, 0, 143, time.Now().UTC()
			_ = m.states.Save(current)
		}
		m.mu.Unlock()
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cur, e := m.Get(id)
		if e != nil || cur.Status == StatusExited {
			return cur, e
		}
		time.Sleep(20 * time.Millisecond)
	}
	return m.Get(id)
}
func (m *Manager) Remove(id string) error {
	c, err := m.Get(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if c.Status == StatusRunning || c.Status == StatusStopping {
		if _, err = m.Stop(id, 10*time.Second); err != nil {
			return err
		}
	}
	if c.Net != nil {
		_ = m.network.Cleanup(&network.Config{Mode: c.Net.Mode, IP: c.Net.IP, Veth: c.Net.Veth, NSPath: c.Net.NSPath})
	}
	mounts := make([]image.Mount, len(c.Mounts))
	for i, x := range c.Mounts {
		mounts[i] = image.Mount{Device: x.Device, Target: x.Target, ReadOnly: x.ReadOnly}
	}
	if err = m.images.UnmountRootfs(mounts); err != nil {
		return err
	}
	if err = m.states.Delete(c.ID); err != nil {
		return err
	}
	if err = os.RemoveAll(filepath.Join(m.root, "containers", c.ID)); err != nil {
		return err
	}
	_ = os.Remove(c.LogFile)
	m.mu.Lock()
	delete(m.items, c.ID)
	delete(m.processes, c.ID)
	delete(m.statsSamples, c.ID)
	m.mu.Unlock()
	m.publish(Event{Type: "container", Action: "removed", ID: c.ID, Name: c.Name})
	return nil
}
func (m *Manager) Get(id string) (*Container, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, err := m.resolveLocked(id)
	if err != nil {
		return nil, err
	}
	return cloneContainer(c), nil
}
func (m *Manager) List(status Status) []*Container {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*Container{}
	for _, c := range m.items {
		if status == "" || c.Status == status {
			out = append(out, cloneContainer(c))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
func (m *Manager) ImageReferences(name string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := m.creatingImages[name]
	for _, c := range m.items {
		if c.Image == name {
			n++
		}
	}
	return n
}
func (m *Manager) Exec(id string, command []string, s execpkg.Streams) (int, error) {
	c, err := m.Get(id)
	if err != nil {
		return -1, err
	}
	if c.Status != StatusRunning {
		return -1, fmt.Errorf("%w: exec requires a running container", ErrConflict)
	}
	return execpkg.Run(c.Pid, c.Spec.NetworkMode == "host", command, c.Spec.Env, s)
}
func (m *Manager) Logs(id string, lines int) (string, error) {
	c, err := m.Get(id)
	if err != nil {
		return "", err
	}
	f, err := os.Open(c.LogFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return execpkg.Tail(f, lines)
}
func (m *Manager) Stats(id string) (Stats, error) {
	c, err := m.Get(id)
	if err != nil {
		return Stats{}, err
	}
	out := Stats{MemoryLimitMB: c.Spec.Resources.MemoryMB, PidsLimit: c.Spec.Resources.PidsLimit}
	if c.Status != StatusRunning || c.Cgroup == nil {
		return out, nil
	}
	cg, err := cgroup.New("/sys/fs/cgroup", c.ID)
	if err != nil {
		return out, err
	}
	s, err := cg.Stats()
	if err != nil {
		return out, err
	}
	out.MemoryUsedMB = s.MemoryBytes / 1024 / 1024
	out.PidsCurrent = s.PidsCurrent
	now := time.Now()
	m.mu.Lock()
	prev, ok := m.statsSamples[c.ID]
	m.statsSamples[c.ID] = cpuSample{usage: s.CPUUsageNS, at: now}
	m.mu.Unlock()
	if ok && s.CPUUsageNS >= prev.usage {
		wall := now.Sub(prev.at).Nanoseconds()
		if wall > 0 {
			out.CPUUsagePercent = float64(s.CPUUsageNS-prev.usage) / float64(wall) * 100
			if out.CPUUsagePercent > float64(runtime.NumCPU()*100) {
				out.CPUUsagePercent = float64(runtime.NumCPU() * 100)
			}
		}
	}
	return out, nil
}
func (m *Manager) Wait(ctx context.Context, id string) (int, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		c, err := m.Get(id)
		if err != nil {
			return -1, err
		}
		if c.Status == StatusExited {
			return c.ExitCode, nil
		}
		if c.Status == StatusCreated {
			return -1, fmt.Errorf("%w: wait requires a started container", ErrConflict)
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
		}
	}
}
func (m *Manager) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	m.mu.Lock()
	m.subscribers[ch] = struct{}{}
	m.mu.Unlock()
	return ch, func() {
		m.mu.Lock()
		if _, ok := m.subscribers[ch]; ok {
			delete(m.subscribers, ch)
			close(ch)
		}
		m.mu.Unlock()
	}
}
func (m *Manager) publish(e Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for ch := range m.subscribers {
		select {
		case ch <- e:
		default:
		}
	}
}
