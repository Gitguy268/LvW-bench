//go:build windows

package backend

import (
	"os/exec"
	"strconv"
	"syscall"
)

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// stop ends the process tree (uv starts python as a child).
func stop(cmd *exec.Cmd) {
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
