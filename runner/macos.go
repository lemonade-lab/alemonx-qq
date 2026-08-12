//go:build darwin

package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	destination, err := macInstallerArchivePath()
	if err != nil {
		return "", err
	}
	downloads := filepath.Dir(destination)
	if err := os.MkdirAll(downloads, 0o700); err != nil {
		return "", err
	}
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
	return fmt.Sprintf("✓ 安装器已下载并校验（%s）。\n文件位置：%s\n下一步：点击「打开安装器」，按 App 内提示完成安装。", tag, destination)
}

func macInstallerArchivePath() (string, error) {
	root, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "downloads", macInstallerAsset), nil
}

func macInstallerReady() bool {
	archive, err := macInstallerArchivePath()
	if err != nil {
		return false
	}
	info, err := os.Stat(archive)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= maxNapcatArchiveSize
}

func macInstallerPath() string {
	archive, err := macInstallerArchivePath()
	if err != nil || !macInstallerReady() {
		return ""
	}
	return archive
}

func macNapcatLauncherPath() string {
	archive, err := macInstallerArchivePath()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(filepath.Dir(archive), "NapCatInstaller", "NapCatInstaller.app")
	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		return ""
	}
	return candidate
}

func openMacNapcatLauncher() (string, error) {
	launcher := macNapcatLauncherPath()
	if launcher == "" {
		return "", fmt.Errorf("未找到 NapCat 启动器；请先点击「安装 NapCat」下载官方安装器")
	}
	if err := exec.Command("open", launcher).Run(); err != nil {
		return "", fmt.Errorf("无法打开 NapCat 启动器：%w", err)
	}
	return "✓ NapCat 启动器已打开。请在启动器中安装、启动 NapCat 或切换原版 QQ。", nil
}

// openMacNapcatInstaller expands the already verified archive to a
// workbench-owned directory and launches its sole app bundle. It deliberately
// uses Go's ZIP reader rather than Archive Utility or a shell command, so the
// location and extracted paths remain bounded and predictable.
func openMacNapcatInstaller() (string, error) {
	archive, err := macInstallerArchivePath()
	if err != nil || !macInstallerReady() {
		return "", fmt.Errorf("安装器尚未下载完成；请先点击「安装 NapCat」")
	}
	root := filepath.Join(filepath.Dir(archive), "NapCatInstaller")
	temporary, err := os.MkdirTemp(filepath.Dir(archive), ".NapCatInstaller-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	app, err := extractMacInstallerApp(archive, temporary)
	if err != nil {
		return "", err
	}
	_ = os.RemoveAll(root)
	if err := os.Rename(temporary, root); err != nil {
		return "", err
	}
	app = filepath.Join(root, strings.TrimPrefix(app, temporary+string(filepath.Separator)))
	if err := exec.Command("open", app).Run(); err != nil {
		return "", fmt.Errorf("无法打开 NapCat 安装器：%w", err)
	}
	return "✓ NapCat 安装器已打开。请按 App 内提示完成安装；完成后回到这里继续。", nil
}

func extractMacInstallerApp(archive, destination string) (string, error) {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return "", fmt.Errorf("安装器压缩包无效：%w", err)
	}
	defer reader.Close()
	var written int64
	appRoots := map[string]bool{}
	for _, item := range reader.File {
		name := filepath.Clean(filepath.FromSlash(item.Name))
		if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("安装器压缩包包含不安全路径")
		}
		if index := strings.Index(filepath.ToSlash(name), ".app/"); index >= 0 {
			appRoots[filepath.FromSlash(filepath.ToSlash(name)[:index+4])] = true
		}
		target := filepath.Join(destination, name)
		rel, err := filepath.Rel(destination, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("安装器压缩包路径无效")
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return "", err
			}
			continue
		}
		if item.UncompressedSize64 > uint64(maxNapcatExtractedSize-written) {
			return "", fmt.Errorf("安装器解压后超过 %d MB 限制", maxNapcatExtractedSize>>20)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return "", err
		}
		input, err := item.Open()
		if err != nil {
			return "", err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, item.Mode()&0o755)
		if err == nil {
			var count int64
			count, err = io.Copy(output, io.LimitReader(input, maxNapcatExtractedSize-written+1))
			written += count
			if written > maxNapcatExtractedSize {
				err = fmt.Errorf("安装器解压后超过 %d MB 限制", maxNapcatExtractedSize>>20)
			}
		}
		closeInput := input.Close()
		var closeOutput error
		if output != nil {
			closeOutput = output.Close()
		}
		if err != nil {
			return "", err
		}
		if closeInput != nil || closeOutput != nil {
			return "", fmt.Errorf("安装器文件写入失败")
		}
	}
	if len(appRoots) != 1 {
		return "", fmt.Errorf("安装器压缩包未包含唯一的 macOS App")
	}
	for app := range appRoots {
		info, err := os.Stat(filepath.Join(destination, app))
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("安装器 App 不完整")
		}
		return filepath.Join(destination, app), nil
	}
	return "", fmt.Errorf("安装器 App 不完整")
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
