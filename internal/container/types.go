package container

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusCreated  Status = "created"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusExited   Status = "exited"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("state conflict")
	ErrInvalidParam = errors.New("invalid parameter")
	ErrImageMissing = errors.New("image missing")
	ErrNotSupported = errors.New("not supported")
)

type Resources struct {
	CPUShares int64   `json:"cpuShares"`
	CPUQuota  float64 `json:"cpuQuota"`
	MemoryMB  int64   `json:"memoryMB"`
	PidsLimit int64   `json:"pidsLimit"`
}
type MountSpec struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}
type Spec struct {
	Cmd         []string    `json:"cmd"`
	Hostname    string      `json:"hostname"`
	NetworkMode string      `json:"networkMode"`
	Resources   Resources   `json:"resources"`
	Mounts      []MountSpec `json:"mounts,omitempty"`
	Env         []string    `json:"env,omitempty"`
}
type MountRecord struct {
	Device   string `json:"device"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}
type NetConfig struct {
	Mode   string `json:"mode"`
	IP     string `json:"ip,omitempty"`
	Veth   string `json:"veth,omitempty"`
	NSPath string `json:"nsPath,omitempty"`
}
type CgroupConfig struct {
	Version int    `json:"version"`
	Path    string `json:"path"`
}
type Container struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Image      string        `json:"image"`
	Spec       Spec          `json:"spec"`
	Status     Status        `json:"status"`
	Pid        int           `json:"pid"`
	Net        *NetConfig    `json:"net,omitempty"`
	Mounts     []MountRecord `json:"mounts,omitempty"`
	Cgroup     *CgroupConfig `json:"cgroup,omitempty"`
	LogFile    string        `json:"logFile"`
	CreatedAt  time.Time     `json:"createdAt"`
	StartedAt  time.Time     `json:"startedAt,omitempty"`
	ExitedAt   time.Time     `json:"exitedAt,omitempty"`
	ExitCode   int           `json:"exitCode"`
	AutoRemove bool          `json:"autoRemove"`
}
type CreateOptions struct {
	Name       string
	Image      string
	Spec       Spec
	AutoRemove bool
}
type Stats struct {
	CPUUsagePercent float64 `json:"cpuUsagePercent"`
	MemoryUsedMB    int64   `json:"memoryUsedMB"`
	MemoryLimitMB   int64   `json:"memoryLimitMB"`
	PidsCurrent     int64   `json:"pidsCurrent"`
	PidsLimit       int64   `json:"pidsLimit"`
}

func (s Spec) Validate() error {
	if len(s.Cmd) == 0 || strings.TrimSpace(s.Cmd[0]) == "" {
		return fmt.Errorf("%w: command is required", ErrInvalidParam)
	}
	if s.NetworkMode != "" && s.NetworkMode != "bridge" && s.NetworkMode != "host" {
		return fmt.Errorf("%w: network mode must be bridge or host", ErrInvalidParam)
	}
	if s.Resources.CPUShares < 0 || s.Resources.CPUQuota < 0 || s.Resources.MemoryMB < 0 || s.Resources.PidsLimit < 0 {
		return fmt.Errorf("%w: resource limits cannot be negative", ErrInvalidParam)
	}
	for _, m := range s.Mounts {
		if !strings.HasPrefix(m.Source, "/") || !strings.HasPrefix(m.Target, "/") {
			return fmt.Errorf("%w: mount paths must be absolute", ErrInvalidParam)
		}
	}
	return nil
}
func (c *Container) Runtime(now time.Time) int64 {
	if c.StartedAt.IsZero() {
		return 0
	}
	end := now
	if c.Status == StatusExited && !c.ExitedAt.IsZero() {
		end = c.ExitedAt
	}
	n := int64(end.Sub(c.StartedAt).Seconds())
	if n < 0 {
		return 0
	}
	return n
}
