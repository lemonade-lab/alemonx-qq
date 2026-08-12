package main

import "testing"

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

	saved := State{Version: "4.18.18", InstallDir: dir + "/napcat", PID: 1234, RuntimeAsset: "QQ_3.2.31_260710_amd64_01.deb", RuntimeArchiveSHA256: "02f677feb1ce01ed293a3c7761e5dd85bd79936f57dcaa4cdb53178ae30e3d6d"}
	if err := saveState(saved); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	reloaded, err := loadState()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if reloaded.Version != saved.Version || reloaded.InstallDir != saved.InstallDir || reloaded.PID != saved.PID || reloaded.RuntimeAsset != saved.RuntimeAsset || reloaded.RuntimeArchiveSHA256 != saved.RuntimeArchiveSHA256 {
		t.Fatalf("round-trip mismatch: got %+v want %+v", reloaded, saved)
	}
}
