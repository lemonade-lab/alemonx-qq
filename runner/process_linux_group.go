//go:build linux

package main

import (
	"os/exec"
	"syscall"
)

func joinProcessGroup(command *exec.Cmd, processGroupID int) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: processGroupID}
}
