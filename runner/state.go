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
	Architecture          string `json:"architecture,omitempty"`
	MigrationSource       string `json:"migrationSource,omitempty"`
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
	root, err := pluginStoreDir(legacy)
	if err != nil {
		return "", err
	}
	platform := runtime.GOOS + "-" + runtime.GOARCH
	if spec := napcatPlatform(); spec != nil {
		platform = spec.Key
	}
	target := filepath.Join(root, "runtimes", platform)
	if err := migrateUnscopedPlatformLogs(root, target, platform); err != nil {
		return "", err
	}
	return target, nil
}

// migrateUnscopedPlatformLogs marks the old shared store as migrated and
// retains only human-readable logs.  Old state files and program directories
// may contain a different operating system's binaries, so they are never
// copied into a runtime-specific directory.
func migrateUnscopedPlatformLogs(root, target, platform string) error {
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	marker := filepath.Join(target, ".migration-v1.json")
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, name := range []string{"napcat.log", "napcat-operation.log", "luckylillia.log", "luckylillia-operation.log", "snowluma.log", "snowluma-operation.log"} {
		source := filepath.Join(root, name)
		info, err := os.Stat(source)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(target, name), data, 0o600); err != nil {
			return err
		}
	}
	metadata, err := json.Marshal(map[string]string{
		"platform": platform,
		"source":   root,
		"message":  "旧共享运行目录未复用；请在当前平台重新安装 QQ 内核。",
	})
	if err != nil {
		return err
	}
	return os.WriteFile(marker, metadata, 0o600)
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
	if state.Architecture == "" {
		state.Architecture = runtime.GOARCH
	}
	if state.Platform == "" {
		if platform := napcatPlatform(); platform != nil {
			state.Platform = platform.Key
		}
	}
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
