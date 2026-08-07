package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectStatusNotInstalled(t *testing.T) {
	payload := collectStatus(State{})
	if payload.Installed {
		t.Fatal("uninstalled must report installed=false")
	}
	if payload.Error == "" {
		t.Fatal("uninstalled must carry an error reason")
	}
}

func TestCollectStatusInstalledButDead(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = original })

	// Create installDir so Installed becomes true.
	install := filepath.Join(dir, "alx-qq", "napcat")
	if err := os.MkdirAll(install, 0755); err != nil {
		t.Fatal(err)
	}
	payload := collectStatus(State{InstallDir: install, PID: -1})
	if !payload.Installed {
		t.Fatal("installed must report installed=true")
	}
	if payload.Running {
		t.Fatal("dead process must report running=false")
	}
	if payload.Error == "" {
		t.Fatal("dead process must carry an error reason")
	}
}

func TestNeedsRestart(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = original })
	install := filepath.Join(dir, "alx-qq", "napcat")
	if err := os.MkdirAll(install, 0755); err != nil {
		t.Fatal(err)
	}

	if needsRestart(State{InstallDir: "", PID: 123}) {
		t.Fatal("no install dir must not restart")
	}
	if needsRestart(State{InstallDir: install, PID: 0}) {
		t.Fatal("no pid must not restart")
	}
	if needsRestart(State{InstallDir: install, PID: -1}) {
		t.Fatal("negative pid must not restart")
	}
	// A non-existent positive PID should trigger a restart (process is dead).
	if !needsRestart(State{InstallDir: install, PID: 999999999}) {
		t.Fatal("dead pid must trigger restart")
	}
}
