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
	if !state.Managed || !napcatStateVerified(state) || state.InstallDir == "" || !dirExists(state.InstallDir) {
		return false
	}
	if state.PID <= 0 || state.Platform == "darwin-external" {
		return false
	}
	return !isRunning(state)
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
	handle, err := openAppendLog(logFile)
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

func watchdogOnAction(confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "开启守护"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if err := requireManagedNapcat(state, "开启守护"); err != nil {
		return "", err
	}
	if processAlive(state.WatchdogPID) {
		return "? NapCat 守护已经开启。", nil
	}
	pid, err := startWatchdog()
	if err != nil {
		return "", err
	}
	state.WatchdogPID = pid
	if err := saveState(state); err != nil {
		stopWatchdog(pid)
		return "", err
	}
	return "✓ 已开启 NapCat 守护。异常退出后约 15 秒会自动恢复。", nil
}

func watchdogOffAction(confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "关闭守护"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if err := requireManagedNapcat(state, "关闭守护"); err != nil {
		return "", err
	}
	if state.WatchdogPID > 0 {
		stopWatchdog(state.WatchdogPID)
	}
	state.WatchdogPID = 0
	if err := saveState(state); err != nil {
		return "", err
	}
	return "✓ 已关闭 NapCat 守护。", nil
}

// watchdogMain is the entry for the detached watchdog subcommand. It polls
// until the watchdog PID recorded in state no longer matches its own (i.e. the
// watchdog was turned off), then exits.
func watchdogMain() int {
	self := os.Getpid()
	for {
		time.Sleep(watchdogInterval)
		// Lifecycle actions hold this lock across directory replacement and
		// state commits. Never restart an old process while an update is in its
		// stopped/download phase.
		unlock, lockErr := acquireNapcatLifecycleLock()
		if lockErr != nil {
			continue
		}
		state, err := loadState()
		if err != nil {
			unlock()
			continue
		}
		if state.WatchdogPID != self {
			unlock()
			return 0
		}
		if needsRestart(state) {
			if process, startErr := startNapCat(state); startErr == nil {
				state.PID, state.ProcessGroupID = process.PID, process.ProcessGroupID
				_ = saveState(state)
			}
		}
		unlock()
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
