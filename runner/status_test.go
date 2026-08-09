package main

import (
	"encoding/json"
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

func TestNapCatQRCodeActionReadsOnlyKnownCacheFile(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = original })
	install := filepath.Join(dir, "napcat-install")
	if err := os.MkdirAll(filepath.Join(install, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	// PNG signature plus a minimal payload is sufficient for the guarded reader.
	if err := os.WriteFile(filepath.Join(install, "cache", "qrcode.png"), append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("qr")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveState(State{InstallDir: install}); err != nil {
		t.Fatal(err)
	}
	output, err := napcatQRCodeAction()
	if err != nil {
		t.Fatal(err)
	}
	var payload napcatQRCode
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Available || payload.PNG == "" || payload.UpdatedAt == "" {
		t.Fatalf("unexpected QR payload: %+v", payload)
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
