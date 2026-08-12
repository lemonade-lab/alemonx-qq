//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const windowsInstallerAsset = "NapCat.Shell.Windows.OneKey.zip"

// Windows ships a graphical OneKey installer, not a stable background
// launcher API. Keep it in a workbench-owned directory and let the official
// UI own QQ injection, starting and later maintenance.
func windowsInstallerArchivePath() (string, error) {
	root, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "downloads", windowsInstallerAsset), nil
}

func windowsInstallerReady() bool {
	archive, err := windowsInstallerArchivePath()
	if err != nil {
		return false
	}
	info, err := os.Stat(archive)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= maxNapcatArchiveSize
}

func windowsInstallerPath() string {
	archive, err := windowsInstallerArchivePath()
	if err != nil || !windowsInstallerReady() {
		return ""
	}
	return archive
}

func windowsNapcatLauncherPath() string {
	archive, err := windowsInstallerArchivePath()
	if err != nil {
		return ""
	}
	launcher := filepath.Join(filepath.Dir(archive), "NapCatInstaller", "NapCatInstaller.exe")
	info, err := os.Stat(launcher)
	if err != nil || info.IsDir() {
		return ""
	}
	return launcher
}

func downloadWindowsNapcatInstaller() (string, error) {
	release, err := fetchLatest()
	if err != nil {
		return "", err
	}
	asset, err := releaseAssetByName(release, windowsInstallerAsset)
	if err != nil {
		return "", err
	}
	expected := normalizedSHA(asset.Digest)
	if !validSHA(expected) {
		return "", fmt.Errorf("官方 Windows 安装器未提供有效 SHA-256 校验和")
	}
	destination, err := windowsInstallerArchivePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", err
	}
	if actual, err := sha256File(destination); err != nil || !strings.EqualFold(actual, expected) {
		temporary := destination + ".download"
		_ = os.Remove(temporary)
		reportNapcatProgress("download", 20, "下载官方 Windows NapCat 安装器")
		actual, downloadErr := downloadFileLimitedWithProgress(asset.URL, temporary, maxNapcatArchiveSize, napcatDownloadProgress("下载官方 Windows NapCat 安装器", 20, 80))
		if downloadErr != nil {
			_ = os.Remove(temporary)
			return "", downloadErr
		}
		if !strings.EqualFold(actual, expected) {
			_ = os.Remove(temporary)
			return "", fmt.Errorf("Windows 安装器 SHA-256 校验失败")
		}
		if err := os.Rename(temporary, destination); err != nil {
			_ = os.Remove(temporary)
			return "", err
		}
	}
	if err := extractWindowsNapcatInstaller(destination); err != nil {
		return "", err
	}
	reportNapcatProgress("complete", 100, "Windows NapCat 安装器已准备好")
	return fmt.Sprintf("✓ 安装器已下载并校验（%s）。\n文件位置：%s\n下一步：点击「打开 NapCat 启动器」。", release.TagName, windowsNapcatLauncherPath()), nil
}

func openWindowsNapcatLauncher() (string, error) {
	launcher := windowsNapcatLauncherPath()
	if launcher == "" {
		return "", fmt.Errorf("未找到 NapCat 启动器；请先点击「安装 NapCat」")
	}
	command := exec.Command(launcher)
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("无法打开 NapCat 启动器：%w", err)
	}
	return "✓ NapCat 启动器已打开。请在启动器中完成安装、启动和后续管理。", nil
}
