//go:build !darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// startNapCat spawns NapCat detached from the runner's process group
// (Windows and Linux). macOS overrides this in macos.go because NapCat runs
// inside the QQ app there.
func startNapCat(state State) (int, error) {
	command, err := napcatCommand(state)
	if err != nil {
		return 0, err
	}
	logFile, err := logPath()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return 0, err
	}
	handle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, err
	}
	defer handle.Close()
	command.Stdin = nil
	command.Stdout = handle
	command.Stderr = handle
	detachProcess(command)
	if err := command.Start(); err != nil {
		return 0, err
	}
	pid := command.Process.Pid
	_ = command.Process.Release()
	return pid, nil
}

func stopProcess(pid int) {
	stopManagedProcess(pid)
}

// isRunning on Windows/Linux means the tracked PID is alive.
func isRunning(state State) bool {
	if state.ProcessGroupID > 0 {
		return processAlive(state.ProcessGroupID)
	}
	return processAlive(state.PID)
}

// platformInstallDir returns the NapCat install directory on Windows/Linux.
func platformInstallDir() (string, error) {
	return managedInstallDir()
}

// macInstallGuide is unused on non-darwin but referenced by install.go's
// switch; provide a stub so the branch compiles everywhere.
func macInstallGuide() (string, error) {
	return "", fmt.Errorf("当前系统不支持 macOS 安装方式")
}

func macNapcatVersion() string { return "" }

func macQQInstalled() bool    { return false }
func macNapcatInjected() bool { return false }
func macInstallDir() (string, error) {
	return "", fmt.Errorf("当前系统不支持 macOS 安装方式")
}
