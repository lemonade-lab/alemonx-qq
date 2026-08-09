//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
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

// stopManagedProcess only targets the process group created by detachProcess.
// LuckyLillia CLI scripts may spawn children, so killing their leader alone
// would leave the WebUI and QQ helpers behind.
func stopManagedProcess(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGINT)
	time.Sleep(500 * time.Millisecond)
	if aliveProbe(pid) {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}
