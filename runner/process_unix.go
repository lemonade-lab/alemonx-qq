//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detachProcess puts the child in its own process group so killing alx (or the
// runner's parent chain) never takes the NapCat process down with it.
func detachProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// aliveProbe is a signal-0 liveness probe on POSIX systems.
func aliveProbe(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
