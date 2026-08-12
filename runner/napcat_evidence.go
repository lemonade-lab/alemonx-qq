package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// napcatReleaseValidationEvidence is optional release metadata. It may be
// embedded by CI for diagnostics, but local installation never depends on it:
// the official Release Asset digest and installed runtime fingerprint remain
// the authority used by the workbench.
var napcatReleaseValidationEvidence = ""

func napcatStateVerified(state State) bool {
	platform := napcatPlatform()
	if platform == nil || !state.Managed || state.InstallMode != "managed" || state.Platform != platform.Key {
		return false
	}
	expected, err := managedInstallDir()
	return err == nil && filepath.Clean(state.InstallDir) == filepath.Clean(expected)
}

func requireManagedNapcat(state State, action string) error {
	if platform := napcatPlatform(); platform != nil && (platform.Key == "darwin-external" || platform.Key == "windows-external") {
		return errors.New("当前系统由官方 NapCat 启动器管理；请在启动器中" + action)
	}
	if !state.Managed || state.InstallMode != "managed" {
		return errors.New("当前 NapCat 是外部关联实例；工作台不能" + action + "。请使用其原始管理方式")
	}
	if !napcatStateVerified(state) {
		return errors.New("当前 NapCat 不是工作台管理的安装，无法" + action)
	}
	return nil
}

func requireNapcatConfirmation(confirmed bool, action string) error {
	if !confirmed {
		return errors.New("请确认后再" + action)
	}
	return nil
}

func reportNapcatProgress(stage string, percent int, message string) {
	payload, err := json.Marshal(map[string]any{"stage": stage, "percent": percent, "message": message})
	if err == nil {
		_, _ = fmt.Fprintf(os.Stderr, "@alx-progress %s\n", payload)
	}
}
