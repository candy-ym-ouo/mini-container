package namespace

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

type Config struct {
	Rootfs, Hostname string
	HostNetwork      bool
	Command, Env     []string
	Mounts           []Mount
}
type Child struct{ Command *exec.Cmd }

func NewCommand(c Config) (*Child, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("namespace isolation requires Linux")
	}
	if len(c.Command) == 0 {
		return nil, fmt.Errorf("container command is empty")
	}
	data, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("encode init config: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{"--fork", "--kill-child=SIGTERM", "--pid", "--mount", "--uts", "--ipc"}
	if !c.HostNetwork {
		args = append(args, "--net")
	}
	args = append(args, exe, "__init", base64.RawURLEncoding.EncodeToString(data))
	cmd := exec.Command("unshare", args...)
	cmd.Env = append(os.Environ(), c.Env...)
	return &Child{Command: cmd}, nil
}
func RunInit(encoded string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("container init requires Linux")
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	var c Config
	if err = json.Unmarshal(data, &c); err != nil {
		return err
	}
	if len(c.Command) == 0 {
		return fmt.Errorf("container command is empty")
	}
	if c.Hostname == "" {
		c.Hostname = "mini-container"
	}
	if out, e := exec.Command("hostname", c.Hostname).CombinedOutput(); e != nil {
		return fmt.Errorf("set hostname: %w: %s", e, out)
	}
	if err = PrepareRootfs(c.Rootfs, c.Mounts); err != nil {
		return err
	}
	ready, release := os.NewFile(3, "init-ready"), os.NewFile(4, "init-release")
	if ready == nil || release == nil {
		return fmt.Errorf("init handshake descriptors are missing")
	}
	if _, err = ready.Write([]byte{1}); err != nil {
		return err
	}
	_ = ready.Close()
	if _, err = release.Read([]byte{0}); err != nil {
		return err
	}
	_ = release.Close()
	if err = syscall.Chroot(c.Rootfs); err != nil {
		return err
	}
	if err = os.Chdir("/"); err != nil {
		return err
	}
	path, err := exec.LookPath(c.Command[0])
	if err != nil {
		return fmt.Errorf("find command %s: %w", c.Command[0], err)
	}
	env := append([]string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/root", "TERM=xterm"}, c.Env...)
	return syscall.Exec(path, c.Command, env)
}
