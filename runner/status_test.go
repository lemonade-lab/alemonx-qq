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

func TestCollectStatusSeparatesAutomaticSupportFromManagedIdentity(t *testing.T) {
	platform := napcatPlatform()
	if platform == nil || !platform.AutoInstall {
		t.Skip("automatic NapCat install is unavailable on this test platform")
	}
	payload := collectStatus(State{})
	if !payload.Verified || payload.ManagedActions {
		t.Fatalf("new installation status = %#v", payload)
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
	// External / pre-governance installs never participate in the watchdog.
	if needsRestart(State{InstallDir: install, PID: 999999999}) {
		t.Fatal("external dead pid must not trigger restart")
	}
}

func TestNapCatStatusKeepsOneBotAccountsSeparate(t *testing.T) {
	original := userConfigDir
	base := t.TempDir()
	userConfigDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userConfigDir = original })
	install := filepath.Join(base, "napcat")
	configDir := filepath.Join(install, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for qq, port := range map[string]string{"10001": "3101", "10002": "3102"} {
		data := []byte(`{"network":{"websocketServers":[{"enable":true,"port":` + port + `}]}}`)
		if err := os.WriteFile(filepath.Join(configDir, "onebot11_"+qq+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	payload := collectStatus(State{InstallDir: install, SelectedQQ: "10002"})
	if len(payload.Accounts) != 2 || payload.SelectedAccount != "10002" || payload.OneBotURL != "ws://127.0.0.1:3102" {
		t.Fatalf("accounts=%+v selected=%q url=%q", payload.Accounts, payload.SelectedAccount, payload.OneBotURL)
	}
}

func TestNapcatLinuxDependencyPreflight(t *testing.T) {
	lookup := func(name string) (string, error) {
		if name == "apt-get" || name == "Xvfb" {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	status := napcatLinuxDependenciesFor("linux", lookup, func(name string) bool { return name != "libgbm1" })
	if status == nil || !status.Supported || status.Ready || len(status.Missing) != 1 || status.Missing[0] != "libgbm1" {
		t.Fatalf("dependency preflight = %#v", status)
	}
	dnfLookup := func(name string) (string, error) {
		if name == "dnf" || name == "Xvfb" {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	dnf := napcatLinuxDependenciesFor("linux", dnfLookup, func(name string) bool { return name != "gtk3" })
	if dnf == nil || dnf.PackageManager != "dnf" || dnf.Ready || len(dnf.Missing) != 1 || dnf.Missing[0] != "gtk3" {
		t.Fatalf("dnf dependency preflight = %#v", dnf)
	}
	if got := napcatLinuxDependenciesFor("darwin", lookup, func(string) bool { return true }); got != nil {
		t.Fatalf("non-Linux dependencies = %#v, want nil", got)
	}
	unknown := napcatLinuxDependenciesFor("linux", func(string) (string, error) { return "", os.ErrNotExist }, func(string) bool { return false })
	if unknown == nil || !unknown.Supported || unknown.RequiresAuthorization || unknown.SystemPackageAvailable {
		t.Fatalf("unknown Linux should use automatic managed runtime: %#v", unknown)
	}
}
