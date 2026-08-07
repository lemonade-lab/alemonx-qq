package main

import (
	"os"
	"os/exec"
	"time"
)

// Watchdog mode keeps NapCat alive: a detached runner instance polls state and
// restarts NapCat if it died while installed. It is independent of alx.

const watchdogInterval = 15 * time.Second

// needsRestart decides whether the watchdog should bring NapCat back up.
// The guard excludes the case where the watchdog itself is not installed yet
// and avoids restart loops while NapCat is intentionally stopped.
func needsRestart(state State) bool {
	if state.InstallDir == "" || !dirExists(state.InstallDir) {
		return false
	}
	if state.PID <= 0 {
		return false
	}
	return !processAlive(state.PID)
}

// startWatchdog detaches a runner instance running the "watchdog" subcommand.
func startWatchdog() (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, err
	}
	logFile, err := logPath()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(dirOf(logFile), 0755); err != nil {
		return 0, err
	}
	handle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, err
	}
	defer handle.Close()
	command := exec.Command(executable, "watchdog")
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

func stopWatchdog(pid int) {
	stopProcess(pid)
}

// watchdogMain is the entry for the detached watchdog subcommand. It polls
// until the watchdog PID recorded in state no longer matches its own (i.e. the
// watchdog was turned off), then exits.
func watchdogMain() int {
	self := os.Getpid()
	for {
		time.Sleep(watchdogInterval)
		state, err := loadState()
		if err != nil {
			continue
		}
		if state.WatchdogPID != self {
			return 0
		}
		if needsRestart(state) {
			if pid, startErr := startNapCat(state); startErr == nil {
				state.PID = pid
				_ = saveState(state)
			}
		}
	}
}

// dirOf returns the parent directory of a path.
func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
