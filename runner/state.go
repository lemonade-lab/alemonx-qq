package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// State tracks the installed NapCat location and the running process. It lives
// in the user config/data directory because the plugin directory may be
// read-only (for example when installed next to the alx executable).

type State struct {
	Version        string `json:"version,omitempty"`
	InstallDir     string `json:"installDir,omitempty"`
	PID            int    `json:"pid,omitempty"`
	ProcessGroupID int    `json:"processGroupId,omitempty"`
	WatchdogPID    int    `json:"watchdogPid,omitempty"`
	Managed        bool   `json:"managed"`
	Platform       string `json:"platform,omitempty"`
	InstallMode    string `json:"installMode,omitempty"`
	ReleaseTag     string `json:"releaseTag,omitempty"`
	Asset          string `json:"asset,omitempty"`
	ArchiveSHA256  string `json:"archiveSha256,omitempty"`
	Fingerprint    string `json:"fingerprint,omitempty"`
	ValidatedAt    string `json:"validatedAt,omitempty"`
	SelectedQQ     string `json:"selectedQq,omitempty"`
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
		return &napcatPlatformSpec{Key: "windows-amd64", Label: "Windows x64", AutoInstall: true}
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
		return linuxRootlessInstallDir()
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
	// Before release governance existed, installations had no immutable source
	// identity. Treat them as external associations; this prevents an upgrade
	// from deleting or running an arbitrary pre-existing directory.
	if state.InstallDir != "" && (state.Platform == "" || state.Fingerprint == "" || !state.Managed) {
		state.Managed = false
		if state.InstallMode == "" {
			state.InstallMode = "external"
		}
	}
	return state, nil
}

func napcatFingerprint(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("NapCat 安装目录为空")
	}
	candidates := []string{
		filepath.Join(root, "resources", "app", "package.json"),
		filepath.Join(root, "opt", "QQ", "resources", "app", "package.json"),
		filepath.Join(root, "package.json"),
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]), nil
	}
	return "", fmt.Errorf("未找到 NapCat 运行时 package.json")
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
