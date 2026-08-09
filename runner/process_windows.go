//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// detachProcess runs the child with CREATE_NEW_PROCESS_GROUP + DETACHED so it
// survives the alx process terminating.
func detachProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// Windows receives a dedicated process group when launched. taskkill /T is
// scoped to that owned root PID and closes CLI children such as WebUI helpers.
func stopManagedProcess(pid int) {
	if pid <= 0 {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = process.Signal(os.Interrupt)
	time.Sleep(500 * time.Millisecond)
	if aliveProbe(pid) {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	}
	if aliveProbe(pid) {
		_ = process.Kill()
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
