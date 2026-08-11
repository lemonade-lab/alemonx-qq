//go:build darwin

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// macOS runs NapCat by injecting it into the installed QQ app (Electron), so
// there is no detached NapCat process to manage. "start" opens QQ (NapCat runs
// inside it); "stop" quits QQ via AppleScript; running is signalled by the
// 6099 WebUI being reachable.

const (
	macQQApp          = "/Applications/QQ.app"
	macQQContainer    = "Library/Containers/com.tencent.qq/Data"
	macInstallerAsset = "NapCatInstaller.zip"
)

// macNapcatContainer returns the NapCat sandbox container path for QQ.
func macNapcatContainer() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, macQQContainer)
}

func macQQInstalled() bool {
	info, err := os.Stat(macQQApp)
	return err == nil && info.IsDir()
}

// macNapcatInjected reports whether NapCat has been injected into QQ's
// sandbox container (the installer writes loadNapCat.js / napcat files there).
func macNapcatInjected() bool {
	container := macNapcatContainer()
	if container == "" {
		return false
	}
	for _, marker := range []string{"loadNapCat.js", "napcat"} {
		if _, err := os.Stat(filepath.Join(container, marker)); err == nil {
			return true
		}
	}
	return false
}

// macInstallDir is the NapCat directory inside the QQ container.
func macInstallDir() (string, error) {
	container := macNapcatContainer()
	if container == "" {
		return "", fmt.Errorf("无法定位 QQ 沙箱容器")
	}
	dir := filepath.Join(container, "napcat")
	if !dirExists(dir) {
		return dir, nil
	}
	return dir, nil
}

// macNapcatVersion reads the NapCat version from the injected package.json.
func macNapcatVersion() string {
	dir, err := macInstallDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return ""
	}
	return manifest.Version
}

// downloadMacNapcatInstaller keeps the official installer at one predictable
// workbench-owned path. The installer itself remains responsible for the QQ
// injection; after it finishes, napcat-adopt uses macInstallDir directly.
func downloadMacNapcatInstaller() (string, error) {
	if !macQQInstalled() {
		return "", fmt.Errorf("未检测到 QQ；请先从 Mac App Store 安装 QQ，然后再下载 NapCat 安装器")
	}
	release, err := fetchRelease(macInstallerReleaseURL, "NapCat macOS 安装器")
	if err != nil {
		return "", err
	}
	asset, err := releaseAssetByName(release, macInstallerAsset)
	if err != nil {
		return "", err
	}
	expected := normalizedSHA(asset.Digest)
	if !validSHA(expected) {
		return "", fmt.Errorf("官方 macOS 安装器未提供有效 SHA-256 校验和")
	}
	root, err := stateDir()
	if err != nil {
		return "", err
	}
	downloads := filepath.Join(root, "downloads")
	if err := os.MkdirAll(downloads, 0o700); err != nil {
		return "", err
	}
	destination := filepath.Join(downloads, macInstallerAsset)
	if current, err := sha256File(destination); err == nil && strings.EqualFold(current, expected) {
		return macInstallerDownloadResult(destination, release.TagName), nil
	}
	temporary := destination + ".download"
	_ = os.Remove(temporary)
	reportNapcatProgress("download", 20, "下载官方 macOS NapCat 安装器")
	actual, err := downloadFileLimitedWithProgress(asset.URL, temporary, maxNapcatArchiveSize, napcatDownloadProgress("下载官方 macOS NapCat 安装器", 20, 85))
	if err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	if !strings.EqualFold(actual, expected) {
		_ = os.Remove(temporary)
		return "", fmt.Errorf("macOS 安装器 SHA-256 校验失败")
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	reportNapcatProgress("complete", 100, "macOS NapCat 安装器已下载")
	return macInstallerDownloadResult(destination, release.TagName), nil
}

func macInstallerDownloadResult(destination, tag string) string {
	return fmt.Sprintf("✓ macOS NapCat 安装器已下载并校验（%s）。\n文件：%s\n下一步：双击此 ZIP 解压，打开 NapCat安装器.app，点击「安装」并按提示授权。完成后回到这里点击「关联已检测到的实例」。", tag, destination)
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// macInstallGuide returns guidance for installing NapCat on macOS.
func macInstallGuide() (string, error) {
	if !macQQInstalled() {
		return "", fmt.Errorf("未检测到 QQ；请先从 Mac App Store 安装 QQ（≥ 9.9.27），然后重试")
	}
	if macNapcatInjected() {
		version := macNapcatVersion()
		if version != "" {
			return version, nil
		}
		return "injected", nil
	}
	hint := strings.Join([]string{
		"在 macOS 上，NapCat 通过注入 QQ 应用运行（无需 Docker）。",
		"安装步骤：",
		"1. 安装 QQ（Mac App Store，≥ 9.9.27）与 Node.js 18+。",
		"2. 在工作台点击「下载 macOS 安装器」。",
		"3. 打开下载目录中的 NapCatInstaller.zip，解压后运行 NapCat安装器.app 并点击「安装」。",
		"4. 按提示在「系统设置 → 隐私与安全性 → App 管理」中授权，完成后回到工作台关联已检测到的实例。",
	}, "\n")
	return "", fmt.Errorf("%s\n（完成上述步骤后重试本操作）", hint)
}

// macStartNapCat is intentionally unavailable. macOS NapCat is only an
// external association: the workbench never drives QQ through AppleScript.
func macStartNapCat() error {
	return fmt.Errorf("macOS NapCat 是外部关联实例，请从 QQ 自身启动；工作台不会执行外部脚本或自动化 QQ")
}

// macStopNapCat never sends AppleScript to QQ for the same reason.
func macStopNapCat() error {
	return fmt.Errorf("macOS NapCat 是外部关联实例，请从 QQ 自身停止；工作台不会执行外部脚本或自动化 QQ")
}

// startNapCat opens QQ on macOS (NapCat runs inside it). No PID is tracked
// because QQ is a foreground app, not a detached process.
func startNapCat(state State) (napcatProcess, error) {
	if err := macStartNapCat(); err != nil {
		return napcatProcess{}, err
	}
	return napcatProcess{}, nil
}

// stopProcess quits QQ on macOS.
func stopProcess(pid int) {
	_ = macStopNapCat()
}

// isRunning on macOS means the injected NapCat's WebUI is reachable.
func isRunning(state State) bool {
	return webUIBridge() != ""
}

// platformInstallDir on macOS is the NapCat directory inside the QQ container.
func platformInstallDir() (string, error) {
	return macInstallDir()
}
