//go:build linux

package main

import (
	"os"
	"testing"
)

func TestNapcatStopWaitsForWholeManagedProcessGroup(t *testing.T) {
	originalConfigDir := userConfigDir
	originalGroupAlive := napcatManagedGroupAlive
	originalStop := stopNapcatManagedProcess
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() {
		userConfigDir = originalConfigDir
		napcatManagedGroupAlive = originalGroupAlive
		stopNapcatManagedProcess = originalStop
	})
	install, err := managedInstallDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(install, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveState(State{InstallDir: install, Managed: true, InstallMode: "managed", Platform: napcatPlatform().Key, ProcessGroupID: 4242}); err != nil {
		t.Fatal(err)
	}
	alive := true
	napcatManagedGroupAlive = func(state State) bool {
		return state.ProcessGroupID == 4242 && alive
	}
	stopped := 0
	stopNapcatManagedProcess = func(pid int) {
		if pid != 4242 {
			t.Fatalf("stop target = %d, want process group 4242", pid)
		}
		stopped++
		alive = false
	}

	if _, err := stopAction(true); err != nil {
		t.Fatalf("stopAction: %v", err)
	}
	if stopped != 1 {
		t.Fatalf("stop calls = %d, want 1", stopped)
	}
	state, err := loadState()
	if err != nil || state.PID != 0 || state.ProcessGroupID != 0 {
		t.Fatalf("state after stop = %+v, %v", state, err)
	}
}
