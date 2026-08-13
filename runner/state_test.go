package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNapcatGraphicalLauncherPlatformsAreExternal(t *testing.T) {
	windows := napcatPlatformFor("windows", "amd64")
	if windows == nil || windows.Key != "windows-external" || windows.AutoInstall {
		t.Fatalf("Windows OneKey installer contract = %#v", windows)
	}
	mac := napcatPlatformFor("darwin", "arm64")
	if mac == nil || mac.Key != "darwin-external" || mac.AutoInstall {
		t.Fatalf("macOS installer contract = %#v", mac)
	}
}

func TestNapcatLinuxPlatformsRemainManaged(t *testing.T) {
	for _, architecture := range []string{"amd64", "arm64"} {
		platform := napcatPlatformFor("linux", architecture)
		if platform == nil || !platform.AutoInstall || platform.Key != "linux-"+architecture {
			t.Fatalf("Linux %s managed runtime contract = %#v", architecture, platform)
		}
	}
}

func TestStateRoundTrip(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	defer func() { userConfigDir = original }()

	state, err := loadState()
	if err != nil {
		t.Fatalf("empty state must load without error: %v", err)
	}
	if state.Version != "" || state.InstallDir != "" || state.PID != 0 {
		t.Fatalf("expected empty state, got %+v", state)
	}

	saved := State{Version: "4.18.18", InstallDir: dir + "/napcat", PID: 1234, EnvironmentMode: "managed-runtime"}
	if err := saveState(saved); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	reloaded, err := loadState()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if reloaded.Version != saved.Version || reloaded.InstallDir != saved.InstallDir || reloaded.PID != saved.PID || reloaded.EnvironmentMode != saved.EnvironmentMode {
		t.Fatalf("round-trip mismatch: got %+v want %+v", reloaded, saved)
	}
}

func TestLoadStateIgnoresLegacyHashFields(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	defer func() { userConfigDir = original }()
	path, err := statePath()
	if err != nil {
		t.Fatal(err)
	}
	legacy := `{"installDir":"/tmp/Napcat","runtimeAsset":"QQ.deb","runtimeArchiveSha256":"02f677feb1ce01ed293a3c7761e5dd85bd79936f57dcaa4cdb53178ae30e3d6d"}`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.InstallDir != "/tmp/Napcat" || state.EnvironmentMode != "" {
		t.Fatalf("legacy runtime migration = %+v", state)
	}
}
