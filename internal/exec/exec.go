package exec

import (
	"fmt"
	"io"
	osexec "os/exec"
	"runtime"
	"strconv"
	"syscall"
)

type Streams struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

func Run(pid int, hostNetwork bool, command []string, env []string, streams Streams) (int, error) {
	if runtime.GOOS != "linux" {
		return -1, fmt.Errorf("exec in container requires Linux")
	}
	if pid <= 0 || len(command) == 0 {
		return -1, fmt.Errorf("pid and command are required")
	}
	args := []string{"--target", strconv.Itoa(pid), "--pid", "--mount", "--uts", "--ipc", "--wd=/"}
	if !hostNetwork {
		args = append(args, "--net")
	}
	args = append(args, "--", command[0])
	args = append(args, command[1:]...)
	cmd := osexec.Command("nsenter", args...)
	cmd.Env = append([]string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/root", "TERM=xterm"}, env...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = streams.Stdin, streams.Stdout, streams.Stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	if e, ok := err.(*osexec.ExitError); ok {
		if s, ok := e.Sys().(syscall.WaitStatus); ok {
			return s.ExitStatus(), nil
		}
	}
	return -1, err
}
