package api

import (
	"mini-container/internal/container"
	"time"
)

type CreateContainerReq struct {
	Name        string                `json:"name"`
	Image       string                `json:"image"`
	Cmd         []string              `json:"cmd"`
	Hostname    string                `json:"hostname"`
	NetworkMode string                `json:"networkMode"`
	Resources   container.Resources   `json:"resources"`
	Mounts      []container.MountSpec `json:"mounts"`
	Env         []string              `json:"env"`
	AutoRemove  bool                  `json:"autoRemove"`
}
type ExecReq struct {
	Cmd []string `json:"cmd"`
}
type ContainerView struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name"`
	Image     string                  `json:"image"`
	Status    container.Status        `json:"status"`
	Pid       int                     `json:"pid"`
	IP        string                  `json:"ip"`
	Hostname  string                  `json:"hostname"`
	Cmd       []string                `json:"cmd"`
	Resources container.Resources     `json:"resources"`
	Mounts    []container.MountRecord `json:"mounts,omitempty"`
	CreatedAt string                  `json:"createdAt"`
	StartedAt string                  `json:"startedAt,omitempty"`
	ExitedAt  string                  `json:"exitedAt,omitempty"`
	ExitCode  int                     `json:"exitCode"`
	Runtime   int64                   `json:"runtimeSeconds"`
	Stats     *container.Stats        `json:"stats,omitempty"`
}
type SystemInfo struct {
	CgroupVersion int    `json:"cgroupVersion"`
	Kernel        string `json:"kernel"`
	GoVersion     string `json:"goVersion"`
	StateDir      string `json:"stateDir"`
	Containers    int    `json:"containers"`
	Images        int    `json:"images"`
	Subnet        string `json:"subnet"`
}
type ErrorEnvelope struct {
	Error APIError `json:"error"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func view(c *container.Container, stats *container.Stats) ContainerView {
	v := ContainerView{ID: c.ID, Name: c.Name, Image: c.Image, Status: c.Status, Pid: c.Pid, Hostname: c.Spec.Hostname, Cmd: c.Spec.Cmd, Resources: c.Spec.Resources, Mounts: c.Mounts, CreatedAt: c.CreatedAt.Format(time.RFC3339), ExitCode: c.ExitCode, Runtime: c.Runtime(time.Now()), Stats: stats}
	if c.Net != nil {
		v.IP = c.Net.IP
	}
	if !c.StartedAt.IsZero() {
		v.StartedAt = c.StartedAt.Format(time.RFC3339)
	}
	if !c.ExitedAt.IsZero() {
		v.ExitedAt = c.ExitedAt.Format(time.RFC3339)
	}
	return v
}
