package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// napcatReleaseValidationEvidence is optional release metadata. It may be
// embedded by CI for diagnostics, but local installation never depends on it:
// the official Release Asset digest and installed runtime fingerprint remain
// the authority used by the workbench.
var napcatReleaseValidationEvidence = ""

func napcatVerified() bool {
	platform := napcatPlatform()
	return platform != nil && platform.AutoInstall
}

// napcatVerificationReason only describes an actual platform limitation.
func napcatVerificationReason() string {
	platform := napcatPlatform()
	if platform == nil {
		return "当前平台不支持 NapCat"
	}
	if !platform.AutoInstall {
		return fmt.Sprintf("%s 的 NapCat 仅支持关联现有实例，不提供自动安装", platform.Label)
	}
	return ""
}

func napcatStateVerified(state State) bool {
	platform := napcatPlatform()
	if platform == nil || !state.Managed || state.InstallMode != "managed" || state.Platform != platform.Key || state.Fingerprint == "" {
		return false
	}
	expected, err := managedInstallDir()
	if err != nil || filepath.Clean(state.InstallDir) != filepath.Clean(expected) {
		return false
	}
	if state.ReleaseTag == "" || state.Asset == "" || !validSHA(state.ArchiveSHA256) {
		return false
	}
	if platform.Key == "linux-amd64" || platform.Key == "linux-arm64" {
		if state.RuntimeAsset == "" || !validSHA(state.RuntimeArchiveSHA256) || !strings.Contains(state.Asset, "+"+state.RuntimeAsset) {
			return false
		}
	}
	fingerprint, err := napcatFingerprint(state.InstallDir)
	return err == nil && fingerprint == state.Fingerprint
}

func requireManagedNapcat(state State, action string) error {
	if platform := napcatPlatform(); platform != nil && (platform.Key == "darwin-external" || platform.Key == "windows-external") {
		return errors.New("当前系统由官方 NapCat 启动器管理；请在启动器中" + action)
	}
	if !state.Managed || state.InstallMode != "managed" {
		return errors.New("当前 NapCat 是外部关联实例；工作台不能" + action + "。请使用其原始管理方式")
	}
	if !napcatStateVerified(state) {
		return errors.New("当前 NapCat 的受管安装信息或运行文件已变化；为保护现有 QQ 环境，已拒绝" + action + "。请重装或改为关联外部实例")
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
