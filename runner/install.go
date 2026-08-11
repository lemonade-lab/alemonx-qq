package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
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
	latestReleaseURL       = "https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest"
	downloadTimeout        = 30 * time.Minute
	maxNapcatArchiveSize   = int64(300 << 20)
	maxNapcatExtractedSize = int64(500 << 20)
	maxInstallerSize       = int64(4 << 20)
	windowsAsset           = "NapCat.Shell.Windows.OneKey.zip"
)

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest,omitempty"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type napcatInstallation struct {
	Version       string
	InstallDir    string
	ReleaseTag    string
	Asset         string
	ArchiveSHA256 string
	Fingerprint   string
	PreviousDir   string
}

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

func releaseAssetByName(release githubRelease, name string) (releaseAsset, error) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("NapCat 发布包中未找到 %s", name)
}

func assetURL(release githubRelease, name string) (string, error) {
	asset, err := releaseAssetByName(release, name)
	return asset.URL, err
}

func normalizedSHA(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}

func validSHA(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func downloadFileLimited(url, dest string, limit int64) (string, error) {
	client := &http.Client{Timeout: downloadTimeout}
	response, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("下载失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败（%s）", response.Status)
	}
	if response.ContentLength > limit {
		return "", fmt.Errorf("下载包超过 %d MB 限制", limit>>20)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	handle, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer handle.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(handle, hash), io.LimitReader(response.Body, limit+1))
	if err != nil {
		return "", err
	}
	if written > limit {
		return "", fmt.Errorf("下载包超过 %d MB 限制", limit>>20)
	}
	if err := handle.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// downloadFile remains available to the LuckyLillia runner. NapCat callers
// must use downloadFileLimited with an explicit integrity boundary.
func downloadFile(url, dest string) error {
	_, err := downloadFileLimited(url, dest, maxNapcatArchiveSize)
	return err
}

func unzipLimited(srcZip, destDir string) error {
	reader, err := zip.OpenReader(srcZip)
	if err != nil {
		return fmt.Errorf("无法打开下载包：%w", err)
	}
	defer reader.Close()
	var total int64
	for _, file := range reader.File {
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return errors.New("下载包包含符号链接，已拒绝解压")
		}
		total += int64(file.UncompressedSize64)
		if total > maxNapcatExtractedSize {
			return errors.New("解压内容超过 500 MB 限制")
		}
		target := filepath.Join(destDir, file.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(filepath.Separator)) {
			return errors.New("下载包包含越界路径，已中止")
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
		if err != nil {
			_ = source.Close()
			return err
		}
		_, err = io.Copy(out, source)
		closeErr := out.Close()
		_ = source.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func napcatExtractRoot(stage string) string {
	for _, candidate := range []string{stage, filepath.Join(stage, "NapCat.Shell.Windows.OneKey")} {
		if _, err := os.Stat(filepath.Join(candidate, "launcher.bat")); err == nil {
			return candidate
		}
		if _, err := os.Stat(filepath.Join(candidate, "launcher.exe")); err == nil {
			return candidate
		}
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			candidate := filepath.Join(stage, entry.Name())
			if _, err := os.Stat(filepath.Join(candidate, "launcher.bat")); err == nil {
				return candidate
			}
			if _, err := os.Stat(filepath.Join(candidate, "launcher.exe")); err == nil {
				return candidate
			}
		}
	}
	return ""
}

func copyNapcatConfig(source, target string) error {
	from, to := filepath.Join(source, "config"), filepath.Join(target, "config")
	entries, err := os.ReadDir(from)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(to, 0o700); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(from, entry.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(to, entry.Name()), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func installWindowsNapCat(evidence napcatEvidence) (napcatInstallation, error) {
	release, err := fetchLatest()
	if err != nil {
		return napcatInstallation{}, err
	}
	asset, err := releaseAssetByName(release, windowsAsset)
	if err != nil {
		return napcatInstallation{}, err
	}
	if evidence.Tag != release.TagName || evidence.Asset != asset.Name || !validSHA(evidence.ArchiveSHA256) || normalizedSHA(asset.Digest) != strings.ToLower(evidence.ArchiveSHA256) {
		return napcatInstallation{}, errors.New("官方 NapCat Release 与当前平台验证证据不匹配")
	}
	root, err := managedInstallDir()
	if err != nil {
		return napcatInstallation{}, err
	}
	stateRoot, err := stateDir()
	if err != nil {
		return napcatInstallation{}, err
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return napcatInstallation{}, err
	}
	archive := filepath.Join(stateRoot, "napcat-windows.zip")
	reportNapcatProgress("download", 25, "下载已验证的 NapCat Windows Release 包")
	digest, err := downloadFileLimited(asset.URL, archive, maxNapcatArchiveSize)
	if err != nil {
		return napcatInstallation{}, err
	}
	defer os.Remove(archive)
	if !strings.EqualFold(digest, evidence.ArchiveSHA256) {
		return napcatInstallation{}, errors.New("NapCat Release SHA-256 校验失败")
	}
	stage, err := os.MkdirTemp(stateRoot, "napcat-stage-*")
	if err != nil {
		return napcatInstallation{}, err
	}
	defer os.RemoveAll(stage)
	reportNapcatProgress("extract", 50, "安全解压 NapCat Windows Release 包")
	if err := unzipLimited(archive, stage); err != nil {
		return napcatInstallation{}, err
	}
	extracted := napcatExtractRoot(stage)
	if extracted == "" {
		return napcatInstallation{}, errors.New("NapCat Release 缺少 launcher.bat 或 launcher.exe")
	}
	fingerprint, err := napcatFingerprint(extracted)
	if err != nil {
		return napcatInstallation{}, err
	}
	if fingerprint != evidence.RuntimeFingerprint {
		return napcatInstallation{}, errors.New("NapCat 运行时指纹与验证证据不匹配")
	}
	backup := root + ".backup"
	if _, err := os.Stat(backup); err == nil {
		return napcatInstallation{}, errors.New("检测到未清理的 NapCat 备份目录")
	}
	reportNapcatProgress("replace", 75, "原子替换 NapCat 安装目录")
	hadPrevious := dirExists(root)
	if hadPrevious {
		if err := os.Rename(root, backup); err != nil {
			return napcatInstallation{}, err
		}
	}
	rollback := func(cause error) (napcatInstallation, error) {
		_ = os.RemoveAll(root)
		if hadPrevious {
			_ = os.Rename(backup, root)
		}
		return napcatInstallation{}, cause
	}
	if err := os.Rename(extracted, root); err != nil {
		return rollback(err)
	}
	if hadPrevious {
		if err := copyNapcatConfig(backup, root); err != nil {
			return rollback(err)
		}
	}
	installation := napcatInstallation{Version: strings.TrimPrefix(release.TagName, "v"), InstallDir: root, ReleaseTag: release.TagName, Asset: asset.Name, ArchiveSHA256: digest, Fingerprint: fingerprint}
	if hadPrevious {
		installation.PreviousDir = backup
	}
	return installation, nil
}

func linuxInstallerURL(commit string) string {
	return "https://raw.githubusercontent.com/NapNeko/NapCat-Installer/" + commit + "/script/install.sh"
}

// rejectPrivilegedInstaller is deliberately conservative. A setup-plugin
// runner has no controlling terminal and must never forward, read or prompt
// for a sudo password. The reviewed Linux path is rootless: if the pinned
// upstream script contains an attempt to invoke sudo, refuse it before any
// existing managed installation is moved aside.
func rejectPrivilegedInstaller(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		// Ignore whole-line comments, but otherwise treat every shell token
		// named sudo as a privilege request, including $(sudo ...).
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, token := range strings.Fields(line) {
			token = strings.Trim(token, "`$(){}[];|&\\\"'")
			if token == "sudo" {
				return errors.New("已拒绝执行包含 sudo 的 NapCat 安装器：工作台不会读取、传递或交互式请求管理员密码。请先在终端完成系统依赖；Linux 自动安装将在经过真实 E2E 审核的完全 rootless 安装器可用后开放")
			}
		}
	}
	return nil
}

func installLinuxNapCat(evidence napcatEvidence) (napcatInstallation, error) {
	if evidence.InstallerCommit == "" || !validSHA(evidence.InstallerSHA256) {
		return napcatInstallation{}, errors.New("Linux NapCat 验证证据缺少固定安装器 commit 或 SHA-256")
	}
	for _, command := range []string{"bash", "xvfb-run"} {
		if _, err := exec.LookPath(command); err != nil {
			return napcatInstallation{}, fmt.Errorf("缺少 %s；请先在终端安装 Linux 依赖后重试。\n建议：sudo apt-get install -y xvfb libnss3 libgbm1", command)
		}
	}
	stateRoot, err := stateDir()
	if err != nil {
		return napcatInstallation{}, err
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return napcatInstallation{}, err
	}
	script := filepath.Join(stateRoot, "napcat-rootless-install.sh")
	reportNapcatProgress("download", 25, "下载固定 commit 的 NapCat rootless 安装器")
	digest, err := downloadFileLimited(linuxInstallerURL(evidence.InstallerCommit), script, maxInstallerSize)
	if err != nil {
		return napcatInstallation{}, err
	}
	defer os.Remove(script)
	if !strings.EqualFold(digest, evidence.InstallerSHA256) {
		return napcatInstallation{}, errors.New("NapCat rootless 安装器 SHA-256 校验失败")
	}
	if err := rejectPrivilegedInstaller(script); err != nil {
		return napcatInstallation{}, err
	}
	root, err := managedInstallDir()
	if err != nil {
		return napcatInstallation{}, err
	}
	backup := root + ".backup"
	if _, err := os.Stat(backup); err == nil {
		return napcatInstallation{}, errors.New("检测到未清理的 NapCat rootless 备份目录")
	}
	hadPrevious := dirExists(root)
	if hadPrevious {
		if err := os.Rename(root, backup); err != nil {
			return napcatInstallation{}, err
		}
	}
	rollback := func(cause error) (napcatInstallation, error) {
		_ = os.RemoveAll(root)
		if hadPrevious {
			_ = os.Rename(backup, root)
		}
		return napcatInstallation{}, cause
	}
	reportNapcatProgress("install", 60, "以当前用户执行 NapCat rootless 安装")
	output, runErr := exec.Command("bash", script, "--docker", "n", "--cli", "n", "--proxy", "0").CombinedOutput()
	if runErr != nil {
		message := strings.TrimSpace(string(output))
		if strings.Contains(strings.ToLower(message), "sudo") || strings.Contains(strings.ToLower(message), "permission denied") {
			return rollback(fmt.Errorf("NapCat rootless 安装需要系统前置依赖或目录权限；工作台不会读取或传递 sudo 密码。请在终端完成管理员前置安装后重试，例如：sudo apt-get install -y xvfb libnss3 libgbm1\n安装器输出：%s", message))
		}
		return rollback(fmt.Errorf("NapCat rootless 安装失败：%s", message))
	}
	fingerprint, err := napcatFingerprint(root)
	if err != nil {
		return rollback(err)
	}
	if fingerprint != evidence.RuntimeFingerprint {
		return rollback(errors.New("NapCat rootless 运行时指纹与验证证据不匹配"))
	}
	installation := napcatInstallation{Version: "rootless", InstallDir: root, ReleaseTag: evidence.InstallerCommit, Asset: "NapCat-Installer", ArchiveSHA256: digest, Fingerprint: fingerprint}
	if hadPrevious {
		installation.PreviousDir = backup
	}
	return installation, nil
}

// discardNapcatBackup is called only after the new state and (when needed)
// process have been committed. Keeping it until then lets an update recover
// from a failed launch instead of leaving a stopped, half-upgraded runtime.
func discardNapcatBackup(installation napcatInstallation) {
	if installation.PreviousDir != "" {
		_ = os.RemoveAll(installation.PreviousDir)
	}
}

func rollbackNapcatInstallation(installation napcatInstallation) error {
	if installation.PreviousDir == "" {
		return nil
	}
	if err := os.RemoveAll(installation.InstallDir); err != nil {
		return err
	}
	return os.Rename(installation.PreviousDir, installation.InstallDir)
}

func installNapCat() (napcatInstallation, error) {
	evidence, err := napcatEvidenceRecord()
	if err != nil {
		return napcatInstallation{}, errors.New(napcatVerificationReason())
	}
	reportNapcatProgress("prepare", 5, "检查 NapCat 平台契约和验证证据")
	switch runtime.GOOS {
	case "windows":
		return installWindowsNapCat(evidence)
	case "linux":
		return installLinuxNapCat(evidence)
	default:
		return napcatInstallation{}, errors.New("macOS NapCat 仅支持外部关联；工作台不会修改 QQ 注入文件")
	}
}
