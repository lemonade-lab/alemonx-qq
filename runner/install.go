package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

const (
	latestReleaseURL = "https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest"
	// Linux needs to download the official QQ runtime in addition to NapCat.
	// Its package is currently about 190 MB, so this must not use the short
	// timeout appropriate for ordinary API calls. Connection and idle timeouts
	// below still fail a genuinely stalled transfer promptly.
	metadataTimeout        = 60 * time.Second
	downloadTimeout        = 60 * time.Minute
	downloadDialTimeout    = 20 * time.Second
	downloadHeaderTimeout  = 60 * time.Second
	downloadIdleTimeout    = 3 * time.Minute
	maxNapcatArchiveSize   = int64(300 << 20)
	maxNapcatExtractedSize = int64(500 << 20)
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
	Version              string
	InstallDir           string
	ReleaseTag           string
	Asset                string
	ArchiveSHA256        string
	RuntimeAsset         string
	RuntimeArchiveSHA256 string
	Fingerprint          string
	PreviousDir          string
}

func fetchLatest() (githubRelease, error) {
	client := &http.Client{Timeout: metadataTimeout}
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

type downloadProgress func(downloaded, total int64)

// downloadFileLimited downloads one trusted archive while enforcing its size
// limit. It intentionally separates the total transfer budget from connection
// and idle budgets: a slow but continuously progressing Linux QQ download is
// valid, while a frozen connection is not.
func downloadFileLimited(url, dest string, limit int64) (string, error) {
	return downloadFileLimitedWithProgress(url, dest, limit, nil)
}

func downloadFileLimitedWithProgress(url, dest string, limit int64, progress downloadProgress) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("创建下载请求失败：%w", err)
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: downloadDialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		IdleConnTimeout:       45 * time.Second,
		TLSHandshakeTimeout:   downloadDialTimeout,
		ResponseHeaderTimeout: downloadHeaderTimeout,
		ExpectContinueTimeout: time.Second,
	}
	client := &http.Client{Transport: transport}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("下载超过 %s；请检查网络或代理后重试", downloadTimeout.Round(time.Minute))
		}
		return "", fmt.Errorf("下载失败：%w", err)
	}
	idleBody := newIdleReadCloser(response.Body, downloadIdleTimeout)
	response.Body = idleBody
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
	writer := io.MultiWriter(handle, hash)
	if progress != nil {
		writer = io.MultiWriter(handle, hash, &downloadProgressWriter{total: response.ContentLength, report: progress})
	}
	written, err := io.Copy(writer, io.LimitReader(response.Body, limit+1))
	if err != nil {
		if idleBody.timedOut.Load() {
			return "", fmt.Errorf("下载超过 %s 没有收到数据；请检查网络或代理后重试", downloadIdleTimeout.Round(time.Second))
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("下载超过 %s；请检查网络或代理后重试", downloadTimeout.Round(time.Minute))
		}
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

type downloadProgressWriter struct {
	total   int64
	written int64
	report  downloadProgress
}

func (w *downloadProgressWriter) Write(data []byte) (int, error) {
	w.written += int64(len(data))
	w.report(w.written, w.total)
	return len(data), nil
}

type idleReadCloser struct {
	io.ReadCloser
	timer    *time.Timer
	timeout  time.Duration
	timedOut atomic.Bool
}

func newIdleReadCloser(body io.ReadCloser, timeout time.Duration) *idleReadCloser {
	reader := &idleReadCloser{ReadCloser: body, timeout: timeout}
	reader.timer = time.AfterFunc(timeout, func() {
		reader.timedOut.Store(true)
		_ = reader.ReadCloser.Close()
	})
	return reader
}

func (r *idleReadCloser) Read(data []byte) (int, error) {
	n, err := r.ReadCloser.Read(data)
	if n > 0 {
		r.timer.Reset(r.timeout)
	}
	return n, err
}

func (r *idleReadCloser) Close() error {
	if r.timer != nil {
		r.timer.Stop()
	}
	return r.ReadCloser.Close()
}

func napcatDownloadProgress(label string, start, end int) downloadProgress {
	lastPercent := -1
	lastReport := time.Time{}
	return func(downloaded, total int64) {
		percent := start
		if total > 0 {
			percent += int((downloaded * int64(end-start)) / total)
			if percent >= end {
				percent = end - 1
			}
		}
		if percent <= lastPercent && time.Since(lastReport) < 2*time.Second {
			return
		}
		lastPercent, lastReport = percent, time.Now()
		if total > 0 {
			reportNapcatProgress("download", percent, fmt.Sprintf("%s（%s / %s）", label, humanBytes(downloaded), humanBytes(total)))
			return
		}
		reportNapcatProgress("download", percent, fmt.Sprintf("%s（已下载 %s）", label, humanBytes(downloaded)))
	}
}

func humanBytes(size int64) string {
	if size < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(size)/(1<<10))
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
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

func installWindowsNapCat() (napcatInstallation, error) {
	release, err := fetchLatest()
	if err != nil {
		return napcatInstallation{}, err
	}
	asset, err := releaseAssetByName(release, windowsAsset)
	if err != nil {
		return napcatInstallation{}, err
	}
	expectedDigest := normalizedSHA(asset.Digest)
	if !validSHA(expectedDigest) {
		return napcatInstallation{}, errors.New("官方 NapCat Release 未提供有效 SHA-256 校验和")
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
	reportNapcatProgress("download", 25, "下载官方 NapCat Windows Release 包")
	digest, err := downloadFileLimitedWithProgress(asset.URL, archive, maxNapcatArchiveSize, napcatDownloadProgress("下载官方 NapCat Windows Release 包", 25, 50))
	if err != nil {
		return napcatInstallation{}, err
	}
	defer os.Remove(archive)
	if !strings.EqualFold(digest, expectedDigest) {
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
		return napcatInstallation{}, errors.New("NapCat Release 缺少受管原生启动器 launcher.exe")
	}
	fingerprint, err := napcatFingerprint(extracted)
	if err != nil {
		return napcatInstallation{}, err
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

func installLinuxNapCat() (napcatInstallation, error) {
	if _, err := exec.LookPath("Xvfb"); err != nil {
		return napcatInstallation{}, errors.New("缺少 Linux 图形运行依赖 Xvfb；请在工作台点击“安装 NapCat”，授权后会自动补齐并继续")
	}
	release, err := fetchLatest()
	if err != nil {
		return napcatInstallation{}, err
	}
	shellAsset, err := releaseAssetByName(release, linuxShellAsset)
	if err != nil {
		return napcatInstallation{}, err
	}
	shellDigest := normalizedSHA(shellAsset.Digest)
	if !validSHA(shellDigest) {
		return napcatInstallation{}, errors.New("官方 NapCat Linux Release 未提供有效 SHA-256 校验和")
	}
	qqAsset, err := linuxQQReleaseAsset()
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
	shellArchive := filepath.Join(stateRoot, "napcat-shell.zip")
	qqArchive := filepath.Join(stateRoot, qqAsset.Name)
	reportNapcatProgress("download", 20, "下载官方 NapCat Linux Release 包")
	digest, err := downloadFileLimitedWithProgress(shellAsset.URL, shellArchive, maxNapcatArchiveSize, napcatDownloadProgress("下载官方 NapCat Linux Release 包", 20, 35))
	if err != nil {
		return napcatInstallation{}, err
	}
	defer os.Remove(shellArchive)
	if !strings.EqualFold(digest, shellDigest) {
		return napcatInstallation{}, errors.New("NapCat Linux Release SHA-256 校验失败")
	}
	reportNapcatProgress("download", 35, "下载官方 Linux QQ 运行时")
	qqDigest, err := downloadFileLimitedWithProgress(qqAsset.URL, qqArchive, maxNapcatArchiveSize, napcatDownloadProgress("下载官方 Linux QQ 运行时", 35, 55))
	if err != nil {
		return napcatInstallation{}, err
	}
	defer os.Remove(qqArchive)
	if !strings.EqualFold(qqDigest, qqAsset.SHA256) {
		return napcatInstallation{}, errors.New("官方 Linux QQ 运行时 SHA-256 校验失败")
	}
	root, err := managedInstallDir()
	if err != nil {
		return napcatInstallation{}, err
	}
	backup := root + ".backup"
	if _, err := os.Stat(backup); err == nil {
		return napcatInstallation{}, errors.New("检测到未清理的 NapCat 旧安装备份目录")
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
	stage, err := os.MkdirTemp(stateRoot, "napcat-linux-stage-*")
	if err != nil {
		return rollback(err)
	}
	defer os.RemoveAll(stage)
	reportNapcatProgress("extract", 55, "安全解压 NapCat Shell 与 Linux QQ")
	if err := unzipLimited(shellArchive, stage); err != nil {
		return rollback(err)
	}
	switch qqAsset.Kind {
	case "deb":
		err = extractDebQQ(qqArchive, stage)
	case "rpm":
		err = extractRPMQQ(qqArchive, stage)
	default:
		err = errors.New("未知 Linux QQ 安装包格式")
	}
	if err != nil {
		return rollback(err)
	}
	reportNapcatProgress("verify", 68, "写入 NapCat 启动入口")
	if _, err := os.Stat(filepath.Join(stage, "napcat", "napcat.mjs")); err != nil {
		return rollback(errors.New("NapCat Shell Release 缺少 napcat/napcat.mjs 启动模块"))
	}
	if err := patchLinuxQQEntrypoint(stage, root); err != nil {
		return rollback(err)
	}
	fingerprint, err := napcatFingerprint(stage)
	if err != nil {
		return rollback(err)
	}
	if err := os.Rename(stage, root); err != nil {
		return rollback(err)
	}
	if hadPrevious {
		if err := copyNapcatConfig(backup, root); err != nil {
			return rollback(err)
		}
	}
	installation := napcatInstallation{Version: strings.TrimPrefix(release.TagName, "v"), InstallDir: root, ReleaseTag: release.TagName, Asset: shellAsset.Name + "+" + qqAsset.Name, ArchiveSHA256: digest, RuntimeAsset: qqAsset.Name, RuntimeArchiveSHA256: qqDigest, Fingerprint: fingerprint}
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
	reportNapcatProgress("prepare", 5, "检查 NapCat 平台契约")
	switch runtime.GOOS {
	case "windows":
		return installWindowsNapCat()
	case "linux":
		return installLinuxNapCat()
	default:
		return napcatInstallation{}, errors.New("macOS NapCat 仅支持外部关联；工作台不会修改 QQ 注入文件")
	}
}
