//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	windowsInstallerAsset         = "NapCat.Shell.Windows.OneKey.zip"
	verifiedWindowsDownloadMirror = "https://gh-proxy.com"
)

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
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func windowsInstallerPath() string {
	archive, err := windowsInstallerArchivePath()
	if err != nil || !windowsInstallerReady() {
		return ""
	}
	return archive
}

// downloadWindowsNapcatArchive keeps GitHub as the primary source.  Some
// Windows networks return 403 (or block the release redirect) while still
// allowing the release API. In that specific case we may use a transport
// fallback only when GitHub supplied a SHA-256 digest; the archive is checked
// before it can be published or extracted.
func downloadWindowsNapcatArchive(asset releaseAsset, destination string, progress downloadProgress) error {
	if err := downloadFileWithProgress(asset.URL, destination, progress); err == nil {
		return nil
	} else if !isGitHubHostedURL(asset.URL) || asset.Digest == "" {
		return err
	} else {
		primaryErr := err
		fallbackURL := verifiedWindowsDownloadMirror + "/" + asset.URL
		for _, candidate := range assetCandidateURLs(asset.URL) {
			if candidate == fallbackURL {
				return primaryErr
			}
		}
		appendActionDiagnostic("install", "GitHub Windows 安装器直连失败，切换到 SHA-256 校验的备用传输")
		if fallbackErr := downloadFromURLs([]string{fallbackURL}, destination, progress, downloadFileOnce); fallbackErr != nil {
			return fmt.Errorf("GitHub 官方下载失败（%v）；备用传输也失败：%w", primaryErr, fallbackErr)
		}
		return nil
	}
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
	destination, err := windowsInstallerArchivePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", err
	}
	if !windowsInstallerReady() {
		temporary := destination + ".download"
		_ = os.Remove(temporary)
		reportNapcatProgress("download", 20, "下载官方 Windows NapCat 安装器")
		if downloadErr := downloadWindowsNapcatArchive(asset, temporary, napcatDownloadProgress("下载官方 Windows NapCat 安装器", 20, 80)); downloadErr != nil {
			_ = os.Remove(temporary)
			return "", downloadErr
		}
		if err := verifyReleaseAssetDigest(temporary, asset); err != nil {
			_ = os.Remove(temporary)
			return "", err
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
	return fmt.Sprintf("✓ 安装器已准备好（%s）。\n文件位置：%s\n下一步：打开文件所在目录，手动双击 NapCatInstaller.exe。", release.TagName, windowsNapcatLauncherPath()), nil
}

func openWindowsNapcatLauncher() (string, error) {
	launcher := windowsNapcatLauncherPath()
	if launcher == "" {
		return "", fmt.Errorf("未找到 NapCat 启动器；请先点击「安装 NapCat」")
	}
	// Windows NapCat is deliberately user-owned: reveal the verified launcher
	// but never execute it or attempt to supervise its QQ process.
	command := exec.Command("explorer.exe", "/select,"+launcher)
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("无法打开 NapCat 文件所在目录：%w", err)
	}
	return "✓ 已打开 NapCat 文件所在目录。请手动双击 NapCatInstaller.exe 完成安装和启动。", nil
}
