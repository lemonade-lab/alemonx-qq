//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detachProcess runs the child with CREATE_NEW_PROCESS_GROUP + DETACHED so it
// survives the alx process terminating.
func detachProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// aliveProbe is a liveness probe for Windows processes.
func aliveProbe(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(process)
	return true
}
