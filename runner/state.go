package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// State tracks the installed NapCat location and the running process. The
// host-managed plugin store keeps it independent of the replaceable plugin
// code directory and persistent across Docker container replacement.

type State struct {
	Version               string `json:"version,omitempty"`
	InstallDir            string `json:"installDir,omitempty"`
	PID                   int    `json:"pid,omitempty"`
	ProcessGroupID        int    `json:"processGroupId,omitempty"`
	WatchdogPID           int    `json:"watchdogPid,omitempty"`
	Managed               bool   `json:"managed"`
	Platform              string `json:"platform,omitempty"`
	InstallMode           string `json:"installMode,omitempty"`
	ReleaseTag            string `json:"releaseTag,omitempty"`
	Asset                 string `json:"asset,omitempty"`
	EnvironmentMode       string `json:"environmentMode,omitempty"`
	FallbackReason        string `json:"fallbackReason,omitempty"`
	EnvironmentDiagnostic string `json:"environmentDiagnostic,omitempty"`
	SelectedQQ            string `json:"selectedQq,omitempty"`
}

type napcatPlatformSpec struct {
	Key         string
	Label       string
	AutoInstall bool
}

func napcatPlatform() *napcatPlatformSpec { return napcatPlatformFor(runtime.GOOS, runtime.GOARCH) }

func napcatPlatformFor(goos, goarch string) *napcatPlatformSpec {
	switch goos + "/" + goarch {
	case "windows/amd64":
		// The official OneKey archive is a graphical NapCatInstaller.exe.  It
		// owns QQ injection and subsequent lifecycle operations, just like the
		// official macOS launcher; ALX only downloads, verifies and opens it.
		return &napcatPlatformSpec{Key: "windows-external", Label: "Windows x64", AutoInstall: false}
	case "linux/amd64":
		return &napcatPlatformSpec{Key: "linux-amd64", Label: "Linux x64", AutoInstall: true}
	case "linux/arm64":
		return &napcatPlatformSpec{Key: "linux-arm64", Label: "Linux ARM64", AutoInstall: true}
	case "darwin/arm64", "darwin/amd64":
		return &napcatPlatformSpec{Key: "darwin-external", Label: "macOS", AutoInstall: false}
	default:
		return nil
	}
}

// userConfigDir is a seam for tests; production code uses os.UserConfigDir.
var userConfigDir = os.UserConfigDir

// legacyStateDir retains the former user-config location for one-time import.
// New runs use ALX_PLUGIN_STORE when the host provides it.
func legacyStateDir() (string, error) {
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

func stateDir() (string, error) {
	legacy, err := legacyStateDir()
	if err != nil {
		return "", err
	}
	return pluginStoreDir(legacy)
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

func linuxRootlessInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Napcat"), nil
}

func managedInstallDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return installDir()
	case "linux":
		return installDir()
	default:
		return "", fmt.Errorf("当前平台不支持受管 NapCat 安装")
	}
}

func logPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "napcat.log"), nil
}

func napcatOperationLogPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "napcat-operation.log"), nil
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
	// Retain an explicit workbench-managed marker when it still points at the
	// workbench-owned directory. Hashes are diagnostic now; their absence must
	// not strand a previously installed NapCat after this upgrade.
	if state.Managed && state.InstallMode == "" {
		if expected, pathErr := managedInstallDir(); pathErr == nil && filepath.Clean(state.InstallDir) == filepath.Clean(expected) {
			state.InstallMode = "managed"
			if platform := napcatPlatform(); platform != nil && state.Platform == "" {
				state.Platform = platform.Key
			}
		}
	}
	if state.InstallDir != "" && (!state.Managed || state.InstallMode != "managed") {
		state.Managed = false
		state.InstallMode = "external"
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
