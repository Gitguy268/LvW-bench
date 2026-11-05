//go:build !windows

package backend

import (
	"os/exec"
	"syscall"
)

func setProcAttr(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// stop sends SIGTERM to the whole process group (uv starts python as a child).
func stop(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
