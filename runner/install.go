package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

const (
	latestReleaseURL       = "https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest"
	macInstallerReleaseURL = "https://api.github.com/repos/NapNeko/NapCat-Mac-Installer/releases/latest"
	// Linux needs to download the official QQ runtime in addition to NapCat.
	// Its package is currently about 190 MB, so this must not use the short
	// timeout appropriate for ordinary API calls. Connection and idle timeouts
	// below still fail a genuinely stalled transfer promptly.
	metadataTimeout       = 60 * time.Second
	downloadTimeout       = 60 * time.Minute
	downloadDialTimeout   = 20 * time.Second
	downloadHeaderTimeout = 60 * time.Second
	downloadIdleTimeout   = 3 * time.Minute
	webUIStartupTimeout   = 3 * time.Minute
	windowsAsset          = "NapCat.Shell.Windows.OneKey.zip"
)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type napcatInstallation struct {
	Version               string
	InstallDir            string
	ReleaseTag            string
	Asset                 string
	EnvironmentMode       string
	FallbackReason        string
	EnvironmentDiagnostic string
	PreviousDir           string
}

func fetchLatest() (githubRelease, error) {
	return fetchRelease(latestReleaseURL, "NapCat")
}

func fetchRelease(releaseURL, name string) (githubRelease, error) {
	client := officialReleaseHTTPClient(metadataTimeout)
	response, err := client.Get(releaseURL)
	if err != nil {
		return githubRelease{}, fmt.Errorf("无法访问 %s 发布信息：%w", name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("%s 发布信息请求失败（%s）", name, response.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("%s 发布信息解析失败：%w", name, err)
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

type downloadProgress func(downloaded, total int64)

// downloadFileWithProgress downloads a fixed official asset. A transfer may be
// large as long as it keeps making progress; only an incomplete response,
// timeout, or I/O error is retried and eventually reported to the user.
func downloadFileWithProgress(url, dest string, progress downloadProgress) error {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		_ = os.Remove(dest)
		err := downloadFileOnce(url, dest, progress)
		if err == nil {
			return nil
		}
		lastErr = err
		_ = os.Remove(dest)
	}
	if lastErr == nil {
		lastErr = errors.New("download did not start")
	}
	// Keep the final transport/HTTP/I/O cause. The UI can keep its top-level
	// wording friendly, but the operation detail and core log must tell an
	// operator what actually failed after the automatic retry.
	return fmt.Errorf("下载重试后仍未完成：%w", lastErr)
}

func downloadFileOnce(url, dest string, progress downloadProgress) error {
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败：%w", err)
	}
	client := officialReleaseHTTPClient(0)
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("下载超过 %s；请检查网络或代理后重试", downloadTimeout.Round(time.Minute))
		}
		return fmt.Errorf("下载失败：%w", err)
	}
	idleBody := newIdleReadCloser(response.Body, downloadIdleTimeout)
	response.Body = idleBody
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败（%s）", response.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	handle, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer handle.Close()
	writer := io.Writer(handle)
	if progress != nil {
		writer = io.MultiWriter(handle, &downloadProgressWriter{total: response.ContentLength, report: progress})
	}
	written, err := io.Copy(writer, response.Body)
	if err != nil {
		if response.ContentLength >= 0 && written != response.ContentLength {
			return fmt.Errorf("下载内容不完整（已接收 %d / %d 字节）；请检查网络或代理后重试", written, response.ContentLength)
		}
		if idleBody.timedOut.Load() {
			return fmt.Errorf("下载超过 %s 没有收到数据；请检查网络或代理后重试", downloadIdleTimeout.Round(time.Second))
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("下载超过 %s；请检查网络或代理后重试", downloadTimeout.Round(time.Minute))
		}
		return err
	}
	if response.ContentLength >= 0 && written != response.ContentLength {
		return fmt.Errorf("下载内容不完整（已接收 %d / %d 字节）；请检查网络或代理后重试", written, response.ContentLength)
	}
	if err := handle.Sync(); err != nil {
		return err
	}
	return nil
}

// officialReleaseHTTPClient sends official NapCat, LuckyLillia and QQ runtime
// downloads through the host broker when a formal plugin Release provides one.
// The runner never receives the workbench proxy URL or its credentials.
func officialReleaseHTTPClient(timeout time.Duration) *http.Client {
	if endpoint := strings.TrimSpace(os.Getenv("ALX_PLUGIN_DOWNLOAD_BROKER")); endpoint != "" && strings.TrimSpace(os.Getenv("ALX_PLUGIN_DOWNLOAD_TOKEN")) != "" {
		return &http.Client{Timeout: timeout, Transport: pluginDownloadBrokerTransport{endpoint: endpoint, token: os.Getenv("ALX_PLUGIN_DOWNLOAD_TOKEN")}}
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
	return &http.Client{Timeout: timeout, Transport: transport}
}

type pluginDownloadBrokerTransport struct {
	endpoint string
	token    string
}

func (t pluginDownloadBrokerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.Method != http.MethodGet {
		return nil, errors.New("官方下载代理只支持 GET 请求")
	}
	broker, err := http.NewRequestWithContext(request.Context(), http.MethodGet, t.endpoint, nil)
	if err != nil {
		return nil, err
	}
	query := broker.URL.Query()
	query.Set("url", request.URL.String())
	broker.URL.RawQuery = query.Encode()
	broker.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(broker)
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

func downloadFile(url, dest string) error {
	return downloadFileWithProgress(url, dest, nil)
}

func unzipArchive(srcZip, destDir string) error {
	return unzipArchiveWithProgress(srcZip, destDir, nil)
}

// unzipArchiveWithProgress keeps the archive-path protections while exposing
// real entry progress to long-running installers. A Linux QQ install may
// spend minutes expanding the official runtime, so a single static “extract”
// event makes a healthy install look frozen.
func unzipArchiveWithProgress(srcZip, destDir string, progress func(completed, total int)) error {
	reader, err := zip.OpenReader(srcZip)
	if err != nil {
		return fmt.Errorf("无法打开下载包：%w", err)
	}
	defer reader.Close()
	if progress != nil {
		progress(0, len(reader.File))
	}
	for index, file := range reader.File {
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return errors.New("下载包包含符号链接，已拒绝解压")
		}
		target := filepath.Join(destDir, file.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(filepath.Separator)) {
			return errors.New("下载包包含越界路径，已中止")
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			if progress != nil {
				progress(index+1, len(reader.File))
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
		if progress != nil {
			progress(index+1, len(reader.File))
		}
	}
	return nil
}

// startNapcatInstallPulse makes CPU/disk-bound archive expansion observable.
// It reports elapsed time rather than inventing a completion percentage.
func startNapcatInstallPulse(percent int, message string) func() {
	done := make(chan struct{})
	go func() {
		started := time.Now()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-ticker.C:
				reportNapcatProgress("extract", percent, fmt.Sprintf("%s（已处理 %s）", message, now.Sub(started).Round(time.Second)))
			}
		}
	}()
	return func() { close(done) }
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
	if err := downloadFileWithProgress(asset.URL, archive, napcatDownloadProgress("下载官方 NapCat Windows Release 包", 25, 50)); err != nil {
		return napcatInstallation{}, err
	}
	defer os.Remove(archive)
	stage, err := os.MkdirTemp(stateRoot, "napcat-stage-*")
	if err != nil {
		return napcatInstallation{}, err
	}
	defer os.RemoveAll(stage)
	reportNapcatProgress("extract", 50, "安全解压 NapCat Windows Release 包")
	if err := unzipArchive(archive, stage); err != nil {
		return napcatInstallation{}, err
	}
	extracted := napcatExtractRoot(stage)
	if extracted == "" {
		return napcatInstallation{}, errors.New("NapCat Release 缺少受管原生启动器 launcher.exe")
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
	installation := napcatInstallation{Version: strings.TrimPrefix(release.TagName, "v"), InstallDir: root, ReleaseTag: release.TagName, Asset: asset.Name}
	if hadPrevious {
		installation.PreviousDir = backup
	}
	return installation, nil
}

func installLinuxNapCat() (napcatInstallation, error) {
	reportNapcatProgress("prepare", 10, "正在准备 NapCat 运行环境")
	environment, err := prepareLinuxEnvironment(false)
	if err != nil {
		return napcatInstallation{}, err
	}
	release, err := fetchLatest()
	if err != nil {
		return napcatInstallation{}, err
	}
	shellAsset, err := releaseAssetByName(release, linuxShellAsset)
	if err != nil {
		return napcatInstallation{}, err
	}
	qqAsset, err := linuxQQReleaseAssetForEnvironment(environment.Mode)
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
	if err := downloadFileWithProgress(shellAsset.URL, shellArchive, napcatDownloadProgress("下载官方 NapCat Linux Release 包", 20, 35)); err != nil {
		return napcatInstallation{}, err
	}
	defer os.Remove(shellArchive)
	reportNapcatProgress("download", 35, "下载官方 Linux QQ 运行时")
	if err := downloadFileWithProgress(qqAsset.URL, qqArchive, napcatDownloadProgress("下载官方 Linux QQ 运行时", 35, 55)); err != nil {
		return napcatInstallation{}, err
	}
	defer os.Remove(qqArchive)
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
	reportNapcatProgress("extract", 56, "正在展开 NapCat Shell")
	if err := unzipArchiveWithProgress(shellArchive, stage, func(completed, total int) {
		if total > 0 && (completed == total || completed%20 == 0) {
			reportNapcatProgress("extract", 56, fmt.Sprintf("正在展开 NapCat Shell（%d/%d 个文件）", completed, total))
		}
	}); err != nil {
		return rollback(err)
	}
	reportNapcatProgress("extract", 58, "正在展开 Linux QQ 运行环境")
	stopPulse := startNapcatInstallPulse(58, "正在展开 Linux QQ 运行环境")
	switch qqAsset.Kind {
	case "deb":
		err = extractDebQQ(qqArchive, stage)
	case "rpm":
		err = extractRPMQQ(qqArchive, stage)
	default:
		err = errors.New("未知 Linux QQ 安装包格式")
	}
	stopPulse()
	if err != nil {
		return rollback(err)
	}
	reportNapcatProgress("verify", 68, "正在写入 NapCat 启动入口")
	if _, err := napcatShellEntrypoint(stage); err != nil {
		return rollback(err)
	}
	// The Shell module is still in stage at this point. Pass that actual
	// location to the generated QQ loader; after the atomic rename it becomes
	// root. Passing root here used to validate an empty/pre-existing directory
	// and incorrectly reported a missing module for valid official archives.
	if err := patchLinuxQQEntrypoint(stage, stage); err != nil {
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
	installation := napcatInstallation{
		Version:               strings.TrimPrefix(release.TagName, "v"),
		InstallDir:            root,
		ReleaseTag:            release.TagName,
		Asset:                 shellAsset.Name + "+" + qqAsset.Name,
		EnvironmentMode:       environment.Mode,
		FallbackReason:        environment.Reason,
		EnvironmentDiagnostic: environment.Diagnostic,
	}
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
		// A first install has no backup. Removing only its verified, managed
		// target is the transactional rollback equivalent; never touch an
		// arbitrary directory supplied by a browser or an external association.
		if installation.InstallDir != "" {
			return os.RemoveAll(installation.InstallDir)
		}
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
