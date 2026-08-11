//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// startNapCat starts the verified Windows launcher executable. Linux has its
// own Go-managed Xvfb implementation in process_linux.go.
func startNapCat(state State) (napcatProcess, error) {
	command, err := napcatCommand(state)
	if err != nil {
		return napcatProcess{}, err
	}
	logFile, err := logPath()
	if err != nil {
		return napcatProcess{}, err
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return napcatProcess{}, err
	}
	handle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return napcatProcess{}, err
	}
	defer handle.Close()
	command.Stdin = nil
	command.Stdout = handle
	command.Stderr = handle
	detachProcess(command)
	if err := command.Start(); err != nil {
		return napcatProcess{}, err
	}
	pid := command.Process.Pid
	_ = command.Process.Release()
	return napcatProcess{PID: pid, ProcessGroupID: pid}, nil
}

func stopProcess(pid int) {
	stopManagedProcess(pid)
}

// isRunning on Windows means the tracked application runtime is alive.
func isRunning(state State) bool {
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
