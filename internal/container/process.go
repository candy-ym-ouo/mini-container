package container

import (
	"fmt"
	"mini-container/internal/namespace"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Process struct {
	PID         int
	supervisor  int
	done        chan struct{}
	result      processResult
	resultMux   sync.RWMutex
	release     *os.File
	releaseOnce sync.Once
}
type processResult struct{ code int }

func StartProcess(c *Container, root string) (*Process, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("%w: containers can only start on Linux", ErrNotSupported)
	}
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("container start requires root privileges; run with sudo")
	}
	mounts := make([]namespace.Mount, len(c.Spec.Mounts))
	for i, m := range c.Spec.Mounts {
		mounts[i] = namespace.Mount{Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly}
	}
	child, err := namespace.NewCommand(namespace.Config{Rootfs: filepath.Join(root, "rootfs"), Hostname: c.Spec.Hostname, HostNetwork: c.Spec.NetworkMode == "host", Command: c.Spec.Cmd, Env: c.Spec.Env, Mounts: mounts})
	if err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(c.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	child.Command.Stdout, child.Command.Stderr = logFile, logFile
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		logFile.Close()
		return nil, err
	}
	releaseRead, releaseWrite, err := os.Pipe()
	if err != nil {
		readyRead.Close()
		readyWrite.Close()
		logFile.Close()
		return nil, err
	}
	child.Command.ExtraFiles = []*os.File{readyWrite, releaseRead}
	if err = child.Command.Start(); err != nil {
		readyRead.Close()
		readyWrite.Close()
		releaseRead.Close()
		releaseWrite.Close()
		logFile.Close()
		return nil, err
	}
	_ = readyWrite.Close()
	_ = releaseRead.Close()
	p := &Process{supervisor: child.Command.Process.Pid, done: make(chan struct{}), release: releaseWrite}
	go func() {
		err := child.Command.Wait()
		_ = logFile.Close()
		code := 0
		if err != nil {
			code = 127
			if s, ok := child.Command.ProcessState.Sys().(syscall.WaitStatus); ok {
				code = s.ExitStatus()
				if s.Signaled() {
					code = 128 + int(s.Signal())
				}
			}
		}
		p.resultMux.Lock()
		p.result = processResult{code: code}
		p.resultMux.Unlock()
		p.closeRelease()
		close(p.done)
	}()
	_ = readyRead.SetReadDeadline(time.Now().Add(15 * time.Second))
	if _, err = readyRead.Read([]byte{0}); err != nil {
		readyRead.Close()
		_ = p.Kill()
		return nil, err
	}
	_ = readyRead.Close()
	p.PID, err = childPID(p.supervisor)
	if err != nil {
		_ = p.Kill()
		return nil, err
	}
	return p, nil
}
func childPID(supervisor int) (int, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(supervisor), "task", strconv.Itoa(supervisor), "children"))
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(data))
	if len(f) == 0 {
		return 0, fmt.Errorf("container init pid is unavailable")
	}
	return strconv.Atoi(f[0])
}
func (p *Process) Terminate() error {
	q, err := os.FindProcess(p.PID)
	if err != nil {
		return err
	}
	return q.Signal(syscall.SIGTERM)
}
func (p *Process) Kill() error {
	p.closeRelease()
	var first error
	for _, pid := range []int{p.PID, p.supervisor} {
		if pid <= 0 {
			continue
		}
		q, err := os.FindProcess(pid)
		if err == nil {
			err = q.Signal(syscall.SIGKILL)
		}
		if err != nil && first == nil {
			first = err
		}
	}
	return first
}
func (p *Process) Release() error {
	select {
	case <-p.done:
		return fmt.Errorf("container init exited before release")
	default:
	}
	var err error
	p.releaseOnce.Do(func() {
		_, err = p.release.Write([]byte{1})
		if e := p.release.Close(); err == nil {
			err = e
		}
	})
	return err
}
func (p *Process) closeRelease() {
	p.releaseOnce.Do(func() {
		if p.release != nil {
			_ = p.release.Close()
		}
	})
}
func (p *Process) Done() <-chan struct{} { return p.done }
func (p *Process) Result() processResult {
	p.resultMux.RLock()
	defer p.resultMux.RUnlock()
	return p.result
}
