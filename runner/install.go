package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	latestReleaseURL = "https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest"
	downloadTimeout  = 30 * time.Minute
)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// windowsAsset returns the Windows OneKey package (bundles QQ + Node).
const windowsAsset = "NapCat.Shell.Windows.OneKey.zip"

// linuxInstallScripts are the NapCat-Installer URLs (installer manages QQ too).
const (
	linuxScriptCN = "https://nclatest.znin.net/NapNeko/NapCat-Installer/main/script/install.sh"
	linuxScript   = "https://raw.githubusercontent.com/NapNeko/NapCat-Installer/main/script/install.sh"
)

// fetchLatest finds the platform package from the latest NapCat release.
func fetchLatest() (githubRelease, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Get(latestReleaseURL)
	if err != nil {
		return githubRelease{}, fmt.Errorf("无法访问 NapCat 发布信息：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("NapCat 发布信息请求失败（%s）", response.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("NapCat 发布信息解析失败：%w", err)
	}
	return release, nil
}

func assetURL(release githubRelease, name string) (string, error) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset.URL, nil
		}
	}
	return "", fmt.Errorf("NapCat 发布包中未找到 %s", name)
}

// downloadFile streams a URL to dest, showing progress on stderr so the action
// result stays clean (the web UI polls the task for completion).
func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: downloadTimeout}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("下载失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败（%s）", response.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	handle, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer handle.Close()
	_, err = io.Copy(handle, response.Body)
	return err
}

// unzip extracts srcZip into destDir.
func unzip(srcZip, destDir string) error {
	reader, err := zip.OpenReader(srcZip)
	if err != nil {
		return fmt.Errorf("无法打开下载包：%w", err)
	}
	defer reader.Close()
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	for _, file := range reader.File {
		target := filepath.Join(destDir, file.Name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(filepath.Separator)) {
			return errors.New("下载包包含越界路径，已中止")
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			source.Close()
			return err
		}
		_, err = io.Copy(out, source)
		source.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// runInstaller executes a shell script with sudo via bash -c (Linux).
func runInstaller(scriptURL string) error {
	if _, err := exec.LookPath("curl"); err != nil {
		return errors.New("未找到 curl；请先安装")
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return errors.New("Linux 安装 NapCat 需要 sudo 权限")
	}
	script := "napcat-install.sh"
	if err := downloadFile(scriptURL, script); err != nil {
		return err
	}
	output, err := exec.Command("sudo", "bash", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("NapCat 安装脚本执行失败：%s", strings.TrimSpace(string(output)))
	}
	return nil
}

// installNapCat installs NapCat for the current platform and returns the
// detected version. It stores install state via the caller.
func installNapCat() (string, error) {
	switch runtime.GOOS {
	case "windows":
		release, err := fetchLatest()
		if err != nil {
			return "", err
		}
		url, err := assetURL(release, windowsAsset)
		if err != nil {
			return "", err
		}
		dir, err := installDir()
		if err != nil {
			return "", err
		}
		zipPath := filepath.Join(dir, "napcat-install.zip")
		if err := downloadFile(url, zipPath); err != nil {
			return "", err
		}
		if err := unzip(zipPath, dir); err != nil {
			return "", err
		}
		_ = os.Remove(zipPath)
		return strings.TrimPrefix(release.TagName, "v"), nil
	case "linux":
		if err := runInstaller(linuxScript); err != nil {
			return "", err
		}
		return "installed-by-script", nil
	case "darwin":
		guide, err := macInstallGuide()
		if err != nil {
			return "", err
		}
		return guide, nil
	default:
		return "", fmt.Errorf("当前系统暂不支持 NapCat 安装（仅 Windows / Linux）")
	}
}
