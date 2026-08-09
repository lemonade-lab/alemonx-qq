package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// State tracks the installed NapCat location and the running process. It lives
// in the user config/data directory because the plugin directory may be
// read-only (for example when installed next to the alx executable).

type State struct {
	Version     string `json:"version,omitempty"`
	InstallDir  string `json:"installDir,omitempty"`
	PID         int    `json:"pid,omitempty"`
	WatchdogPID int    `json:"watchdogPid,omitempty"`
}

// userConfigDir is a seam for tests; production code uses os.UserConfigDir.
var userConfigDir = os.UserConfigDir

// stateDir returns the base directory for alx-qq state: ~/.alx-qq on Unix,
// %LOCALAPPDATA%\alx-qq on Windows.
func stateDir() (string, error) {
	if runtime.GOOS == "windows" {
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return filepath.Join(base, "alx-qq"), nil
		}
	}
	config, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "alx-qq"), nil
}

func statePath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func installDir() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "napcat"), nil
}

func logPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "napcat.log"), nil
}

func loadState() (State, error) {
	path, err := statePath()
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func saveState(state State) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".new"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
