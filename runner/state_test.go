package main

import "testing"

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

	saved := State{Version: "4.18.18", InstallDir: dir + "/napcat", PID: 1234}
	if err := saveState(saved); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	reloaded, err := loadState()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if reloaded.Version != saved.Version || reloaded.InstallDir != saved.InstallDir || reloaded.PID != saved.PID {
		t.Fatalf("round-trip mismatch: got %+v want %+v", reloaded, saved)
	}
}
