package main

import (
	"archive/tar"
	"archive/zip"
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
	"strconv"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

const (
	luckyReleaseURL = "https://api.github.com/repos/LLOneBot/LuckyLilliaBot/releases/latest"
	luckyWebUIPort  = 3080
	luckyOneBotPort = 7199
)

type luckyState struct {
	Version        string `json:"version,omitempty"`
	InstallDir     string `json:"installDir,omitempty"`
	PID            int    `json:"pid,omitempty"`
	ProcessGroupID int    `json:"processGroupId,omitempty"`
	Managed        bool   `json:"managed"`
	Platform       string `json:"platform,omitempty"`
	InstallMode    string `json:"installMode,omitempty"`
	ReleaseTag     string `json:"releaseTag,omitempty"`
	Asset          string `json:"asset,omitempty"`
}

// luckyProcess records the CLI leader and the process group owned by the
// workbench. Linux uses an Xvfb leader so the display server and LLBot are
// stopped together; other platforms use the CLI process itself as the group.
type luckyProcess struct {
	PID            int
	ProcessGroupID int
}

type kernelStatus struct {
	Engine         string         `json:"engine"`
	Installed      bool           `json:"installed"`
	InstallHealthy bool           `json:"installHealthy"`
	Running        bool           `json:"running"`
	PortReachable  bool           `json:"portReachable"`
	WebUIReady     bool           `json:"webUiReady"`
	OneBotReady    bool           `json:"oneBotReady"`
	LoginPending   bool           `json:"loginPending"`
	Version        string         `json:"version,omitempty"`
	PID            int            `json:"pid,omitempty"`
	WebUIURL       string         `json:"webUiUrl,omitempty"`
	OneBotURL      string         `json:"oneBotUrl,omitempty"`
	LogPath        string         `json:"logPath,omitempty"`
	DiagnosticHint string         `json:"diagnosticHint,omitempty"`
	Error          string         `json:"error,omitempty"`
	Supported      bool           `json:"supported"`
	Platform       string         `json:"platform,omitempty"`
	InstallMode    string         `json:"installMode,omitempty"`
	Managed        bool           `json:"managed"`
	AuthTokenReady bool           `json:"authTokenReady"`
	State          string         `json:"state"`
	UpdatedAt      string         `json:"updatedAt"`
	Journey        runtimeJourney `json:"journey"`
}

// luckyPlatformSpec describes the official CLI contract for each supported
// platform. Installation validates its unpacked structure and actual startup.
type luckyPlatformSpec struct {
	Key         string
	Label       string
	AssetName   string
	Archive     string
	Entrypoint  string
	CLIBinary   string
	AutoInstall bool
}

func luckyPlatform() *luckyPlatformSpec { return luckyPlatformFor(runtime.GOOS, runtime.GOARCH) }

func luckyPlatformFor(goos, goarch string) *luckyPlatformSpec {
	switch goos + "/" + goarch {
	case "linux/arm64":
		return &luckyPlatformSpec{Key: "linux-arm64", Label: "Linux ARM64", AssetName: "LLBot-CLI-linux-arm64.zip", Archive: "zip", Entrypoint: "start.sh", CLIBinary: "llbot", AutoInstall: true}
	case "linux/amd64":
		return &luckyPlatformSpec{Key: "linux-amd64", Label: "Linux x64", AssetName: "LLBot-CLI-linux-x64.zip", Archive: "zip", Entrypoint: "start.sh", CLIBinary: "llbot", AutoInstall: true}
	case "windows/amd64":
		return &luckyPlatformSpec{Key: "windows-amd64", Label: "Windows x64", AssetName: "LLBot-CLI-win-x64.zip", Archive: "zip", Entrypoint: "llbot.exe", CLIBinary: "llbot.exe", AutoInstall: true}
	case "darwin/arm64":
		return &luckyPlatformSpec{Key: "darwin-arm64", Label: "macOS Apple Silicon", AssetName: "LLBot-CLI-macos-arm64.tar.xz", Archive: "tar.xz", Entrypoint: "start.sh", CLIBinary: "llbot", AutoInstall: true}
	default:
		return nil
	}
}

func luckyStatePath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "luckylillia-state.json"), nil
}
func luckyInstallDir() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "luckylillia"), nil
}
func luckyLogPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "luckylillia.log"), nil
}
func loadLuckyState() (luckyState, error) {
	path, err := luckyStatePath()
	if err != nil {
		return luckyState{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return luckyState{}, nil
	}
	if err != nil {
		return luckyState{}, err
	}
	var state luckyState
	if err := json.Unmarshal(data, &state); err != nil {
		return luckyState{}, err
	}
	// Preserve an explicit historical managed marker. Content hashes are now
	// diagnostic only, so their absence must not turn a workbench-owned install
	// into an unusable external association after an upgrade.
	if state.Managed && state.InstallMode == "" {
		if expected, pathErr := luckyInstallDir(); pathErr == nil && filepath.Clean(state.InstallDir) == filepath.Clean(expected) {
			state.InstallMode = "managed"
			if platform := luckyPlatform(); platform != nil && state.Platform == "" {
				state.Platform = platform.Key
			}
		}
	}
	if state.InstallDir != "" && (!state.Managed || state.InstallMode != "managed") {
		state.Managed = false
		state.InstallMode = "external"
	}
	return state, nil
}
func saveLuckyState(state luckyState) error {
	path, err := luckyStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".new", append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(path+".new", path)
}

func luckySupported() bool { return luckyPlatform() != nil }

func luckyManagedState(state luckyState) bool {
	platform := luckyPlatform()
	if platform == nil || !state.Managed || state.InstallMode != "managed" || state.Platform != platform.Key {
		return false
	}
	expected, err := luckyInstallDir()
	return err == nil && filepath.Clean(state.InstallDir) == filepath.Clean(expected)
}

func requireLuckyConfirmation(confirmed bool, action string) error {
	if !confirmed {
		return errors.New("请确认后再" + action)
	}
	return nil
}

func requireManagedLucky(state luckyState, action string) error {
	if !state.Managed || state.InstallMode != "managed" {
		return errors.New("当前 LuckyLillia 是外部关联实例；工作台不能" + action)
	}
	if !luckyManagedState(state) {
		return errors.New("当前 LuckyLillia 不是工作台管理的安装，无法" + action)
	}
	return nil
}

func reportLuckyProgress(stage string, percent int, message string) {
	appendActionDiagnostic(currentLuckyOperationAction(), fmt.Sprintf("[%s] %d%% %s", time.Now().UTC().Format(time.RFC3339), percent, message))
	if strings.TrimSpace(os.Getenv("ALX_PLUGIN_PROGRESS_MODE")) != "structured" {
		fmt.Fprintf(os.Stderr, "\r\033[2K[%3d%%] %s", percent, message)
		return
	}
	payload, err := json.Marshal(map[string]any{"stage": stage, "percent": percent, "message": message})
	if err == nil {
		_, _ = fmt.Fprintf(os.Stderr, "@alx-progress %s\n", payload)
	}
}
func luckyPortURL(port int) string {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 350*time.Millisecond)
	if err != nil {
		return ""
	}
	_ = connection.Close()
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func luckyStatus() (string, error) {
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	installDir := state.InstallDir
	if installDir == "" {
		installDir, _ = luckyInstallDir()
	}
	entry := luckyEntryPoint(installDir)
	installed := installDir != "" && dirExists(installDir)
	healthy := installed && entry != ""
	authTokenReady := luckyAuthTokenReady(installDir)
	running := processAlive(state.PID)
	webPort, oneBotPort := luckyConfiguredPorts()
	webUI := luckyPortURL(webPort)
	onebot := luckyPortURL(oneBotPort)
	stateName := "not-installed"
	if installed {
		stateName = "stopped"
	}
	if installed && !authTokenReady {
		stateName = "needs-auth-token"
	}
	if running {
		stateName = "running"
	}
	if running && webUI != "" && onebot == "" {
		stateName = "login-pending"
	}
	platform := luckyPlatform()
	if !luckySupported() {
		stateName = "unsupported"
	}
	status := kernelStatus{Engine: "luckylillia", Installed: installed, InstallHealthy: healthy, Running: running, PortReachable: webUI != "", WebUIReady: webUI != "", OneBotReady: onebot != "", LoginPending: running && webUI != "" && onebot == "", Version: state.Version, PID: state.PID, WebUIURL: webUI, OneBotURL: "ws://127.0.0.1:" + strconv.Itoa(oneBotPort), Supported: luckySupported(), Managed: state.Managed, AuthTokenReady: authTokenReady, InstallMode: state.InstallMode, State: stateName, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if platform != nil {
		status.Platform = platform.Key
	}
	if path, logErr := luckyLogPath(); logErr == nil {
		status.LogPath = path
	}
	if !status.Supported {
		status.DiagnosticHint = "当前系统尚未纳入 LuckyLillia 的工作台适配范围。"
	}
	if installed && !state.Managed {
		status.DiagnosticHint = "这是外部关联的 LuckyLillia CLI；工作台仅提供状态与 WebUI，不能启动、更新、写配置或卸载。"
	}
	if installed && !healthy {
		status.Error, status.DiagnosticHint = "安装目录不完整", "缺少 LuckyLillia 启动入口，请执行重装。"
	} else if installed && !authTokenReady {
		status.Error, status.DiagnosticHint = "缺少 Auth Token", "请从 auth.luckylillia.com 获取 Token，并在“网络配置”中保存后再启动。"
	}
	if running && webUI == "" {
		status.DiagnosticHint = "进程已启动但 WebUI 未就绪，请查看日志。"
	}
	status.Journey = luckyJourney(status)
	data, err := json.Marshal(status)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func currentLuckyOperationAction() string {
	action := currentOperationAction()
	if strings.HasPrefix(action, "luckylillia-") {
		return action
	}
	return "luckylillia-install"
}

func luckyJourney(status kernelStatus) runtimeJourney {
	switch {
	case !status.Supported:
		return runtimeJourney{Phase: "unsupported", Title: "当前系统暂不支持", Detail: firstStatusDetail(status.DiagnosticHint, "请手动安装后再关联 LuckyLillia。"), NextAction: "manual"}
	case !status.Installed:
		return runtimeJourney{Phase: "install", Title: "安装 LuckyLillia", Detail: "将下载并验证官方 CLI；安装后还需填写官方 Auth Token。", NextAction: "install"}
	case !status.InstallHealthy:
		return runtimeJourney{Phase: "repair", Title: "LuckyLillia 安装不完整", Detail: firstStatusDetail(status.DiagnosticHint, "请重新安装后再启动。"), NextAction: "repair"}
	case !status.Managed:
		return runtimeJourney{Phase: "external", Title: "LuckyLillia 已关联", Detail: firstStatusDetail(status.DiagnosticHint, "这是外部实例；工作台不会修改其进程或配置。"), NextAction: "open-webui"}
	case !status.AuthTokenReady:
		return runtimeJourney{Phase: "needs-auth-token", Title: "需要 Auth Token", Detail: "从 auth.luckylillia.com 获取 Token，在网络配置中保存后才可启动 WebUI。", NextAction: "auth-token"}
	case !status.Running:
		return runtimeJourney{Phase: "start", Title: "启动 LuckyLillia", Detail: "启动后将等待 WebUI，并进入 QQ 登录流程。", NextAction: "start"}
	case !status.WebUIReady:
		return runtimeJourney{Phase: "starting", Title: "正在启动 LuckyLillia", Detail: firstStatusDetail(status.DiagnosticHint, "进程已启动，正在等待管理页面就绪。"), NextAction: "view-log"}
	case status.LoginPending:
		return runtimeJourney{Phase: "scan-qq", Title: "请在管理页面登录 QQ", Detail: "完成登录后 OneBot 服务会自动继续初始化。", NextAction: "open-webui"}
	case !status.OneBotReady:
		return runtimeJourney{Phase: "connecting", Title: "正在等待 OneBot", Detail: "管理页面已就绪，正在等待已配置的 OneBot 服务监听端口。", NextAction: "view-log"}
	default:
		return runtimeJourney{Phase: "ready", Title: "LuckyLillia 已就绪", Detail: "QQ 与 OneBot 服务均已可用，可同步到机器人。", NextAction: "configure"}
	}
}

func luckyRelease() (githubRelease, error) {
	response, err := officialReleaseHTTPClient(20 * time.Second).Get(luckyReleaseURL)
	if err != nil {
		return githubRelease{}, fmt.Errorf("无法访问 LuckyLillia 发布信息：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("LuckyLillia 发布信息请求失败（%s）", response.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&release); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func requireLuckySupported(action string) error {
	platform := luckyPlatform()
	if platform == nil {
		return errors.New("当前系统不支持 LuckyLillia CLI 自动" + action)
	}
	if !platform.AutoInstall {
		return fmt.Errorf("%s 暂不支持 LuckyLillia CLI 自动%s", platform.Label, action)
	}
	return nil
}

func luckyReleaseAsset(release githubRelease) (releaseAsset, error) {
	platform := luckyPlatform()
	if platform == nil || platform.AssetName == "" {
		return releaseAsset{}, errors.New("当前平台没有可用的 LuckyLillia 官方 Release Asset")
	}
	for _, asset := range release.Assets {
		if asset.Name != platform.AssetName {
			continue
		}
		return asset, nil
	}
	return releaseAsset{}, fmt.Errorf("LuckyLillia 发布包中未找到 %s", platform.AssetName)
}

func luckyInstall(force, confirmed bool) (string, error) {
	if err := requireLuckyConfirmation(confirmed, "安装 LuckyLillia"); err != nil {
		return "", err
	}
	if err := requireLuckySupported("安装"); err != nil {
		return "", err
	}
	previous, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if !force {
		if previous.InstallDir != "" && luckyEntryPoint(previous.InstallDir) != "" {
			return "? LuckyLillia 已安装；如需覆盖安装请使用重装。", nil
		}
	}
	if force && !previous.Managed {
		return "", errors.New("外部关联安装不能重装；请先取消关联，再使用工作台安装")
	}
	wasRunning := force && processAlive(previous.PID)
	restartPrevious := func() {
		if !wasRunning {
			return
		}
		_ = saveLuckyState(previous)
		_, _ = luckyStart(true)
	}
	if wasRunning {
		if _, err := luckyStop(true); err != nil {
			return "", fmt.Errorf("停止旧 LuckyLillia 失败，未开始更新：%w", err)
		}
	}
	reportLuckyProgress("prepare", 5, "准备 LuckyLillia CLI 安装")
	release, err := luckyRelease()
	if err != nil {
		restartPrevious()
		return "", err
	}
	asset, err := luckyReleaseAsset(release)
	if err != nil {
		restartPrevious()
		return "", err
	}
	base, err := stateDir()
	if err != nil {
		restartPrevious()
		return "", err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		restartPrevious()
		return "", err
	}
	platform := luckyPlatform()
	archive, err := os.CreateTemp(base, "luckylillia-*"+luckyArchiveSuffix(platform))
	if err != nil {
		restartPrevious()
		return "", err
	}
	archivePath := archive.Name()
	_ = archive.Close()
	defer os.Remove(archivePath)
	reportLuckyProgress("download", 20, "下载官方 CLI 包")
	if err := downloadLuckyAsset(asset, archivePath); err != nil {
		restartPrevious()
		return "", err
	}
	stage, err := os.MkdirTemp(base, ".luckylillia-stage-")
	if err != nil {
		restartPrevious()
		return "", err
	}
	defer os.RemoveAll(stage)
	reportLuckyProgress("extract", 50, "校验并解压官方 CLI 包")
	if err := extractLuckyArchive(archivePath, stage, platform); err != nil {
		restartPrevious()
		return "", err
	}
	root := luckyExtractRoot(stage)
	reportLuckyProgress("verify", 65, "验证 CLI 启动入口")
	if luckyEntryPoint(root) == "" {
		restartPrevious()
		return "", errors.New("LuckyLillia 安装包缺少可用启动入口")
	}
	target, err := luckyInstallDir()
	if err != nil {
		restartPrevious()
		return "", err
	}
	backup := target + ".previous"
	if _, err := os.Stat(backup); err == nil {
		restartPrevious()
		return "", errors.New("检测到未清理的 LuckyLillia 备份目录，请先检查后重试")
	}
	hadTarget := false
	reportLuckyProgress("replace", 75, "原子替换安装目录")
	if _, statErr := os.Stat(target); statErr == nil {
		hadTarget = true
		if err := os.Rename(target, backup); err != nil {
			restartPrevious()
			return "", fmt.Errorf("无法备份旧 LuckyLillia：%w", err)
		}
	}
	if err := os.Rename(root, target); err != nil {
		if _, backupErr := os.Stat(backup); backupErr == nil {
			_ = os.Rename(backup, target)
		}
		restartPrevious()
		return "", fmt.Errorf("替换 LuckyLillia 安装目录失败：%w", err)
	}
	if hadTarget {
		if err := restoreLuckyConfig(backup, target); err != nil {
			_ = os.RemoveAll(target)
			_ = os.Rename(backup, target)
			restartPrevious()
			return "", fmt.Errorf("恢复 LuckyLillia 配置失败，已回滚：%w", err)
		}
	}
	state := luckyState{Version: strings.TrimPrefix(release.TagName, "v"), InstallDir: target, Managed: true, Platform: platform.Key, InstallMode: "managed", ReleaseTag: release.TagName, Asset: asset.Name}
	if err := saveLuckyState(state); err != nil {
		_ = os.RemoveAll(target)
		if hadTarget {
			_ = os.Rename(backup, target)
		}
		restartPrevious()
		return "", err
	}
	// The CLI refuses to start without this token and then waits for stdin.
	// ALX is non-interactive, so retain the verified installation and request
	// the token instead of reporting a fabricated WebUI timeout.
	if !luckyAuthTokenReady(target) {
		reportLuckyProgress("auth", 90, "等待填写 LuckyLillia Auth Token")
		if hadTarget {
			_ = os.RemoveAll(backup)
		}
		return "? LuckyLillia 已安装，但尚未填写 Auth Token。\n? 请在“网络配置”中填写从 https://auth.luckylillia.com 获取的 Token，然后启动。", nil
	}
	// A reinstall retains the private token and can start normally.
	reportLuckyProgress("start", 90, "启动 LuckyLillia 并等待登录")
	if _, err := luckyStart(true); err != nil {
		_ = os.RemoveAll(target)
		if hadTarget {
			_ = os.Rename(backup, target)
		}
		restartPrevious()
		return "", fmt.Errorf("LuckyLillia 启动失败，已恢复安装前状态：%w", err)
	}
	if hadTarget {
		_ = os.RemoveAll(backup)
	}
	reportLuckyProgress("complete", 100, "LuckyLillia CLI 安装完成")
	return fmt.Sprintf("✓ LuckyLillia 已安装并启动（版本 %s）。请进入 WebUI 登录。", state.Version), nil
}

func downloadLuckyAsset(asset releaseAsset, destination string) error {
	return downloadFileWithProgress(asset.URL, destination, napcatDownloadProgress("下载官方 LuckyLillia CLI 包", 20, 50))
}

func luckyArchiveSuffix(platform *luckyPlatformSpec) string {
	if platform != nil && platform.Archive == "tar.xz" {
		return ".tar.xz"
	}
	return ".zip"
}

func extractLuckyArchive(archivePath, destination string, platform *luckyPlatformSpec) error {
	if platform == nil {
		return errors.New("当前平台没有 LuckyLillia CLI 安装契约")
	}
	if platform.Archive == "tar.xz" {
		return extractLuckyTarXZ(archivePath, destination)
	}
	return extractLuckyZip(archivePath, destination)
}

func luckyArchiveTarget(destination, name string) (string, error) {
	name = filepath.Clean(name)
	if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || name == ".." {
		return "", errors.New("安装包包含越界路径")
	}
	target := filepath.Join(destination, name)
	if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destination)+string(filepath.Separator)) {
		return "", errors.New("安装包包含越界路径")
	}
	return target, nil
}

func extractLuckyZip(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("LuckyLillia 安装包无效：%w", err)
	}
	defer reader.Close()
	for _, item := range reader.File {
		target, err := luckyArchiveTarget(destination, item.Name)
		if err != nil {
			return err
		}
		if item.Mode()&os.ModeSymlink != 0 {
			return errors.New("安装包不能包含符号链接")
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := item.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, item.Mode()|0o600)
		if err == nil {
			_, err = io.Copy(output, source)
			_ = output.Close()
		}
		_ = source.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractLuckyTarXZ(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := xz.NewReader(file)
	if err != nil {
		return fmt.Errorf("LuckyLillia tar.xz 安装包无效：%w", err)
	}
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("读取 LuckyLillia tar.xz 失败：%w", err)
		}
		target, err := luckyArchiveTarget(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return errors.New("安装包包含无效文件大小")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode()|0o600)
			if err != nil {
				return err
			}
			count, copyErr := io.Copy(output, io.LimitReader(reader, header.Size))
			closeErr := output.Close()
			if copyErr != nil {
				return copyErr
			}
			if count != header.Size {
				return errors.New("LuckyLillia tar.xz 安装包内容不完整")
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("安装包包含不受支持的链接或特殊文件")
		}
	}
}

func luckyExtractRoot(stage string) string {
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return stage
	}
	return filepath.Join(stage, entries[0].Name())
}

func restoreLuckyConfig(previous, target string) error {
	sourceDir := filepath.Join(previous, "bin", "llbot", "data")
	entries, err := os.ReadDir(sourceDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	targetDir := filepath.Join(target, "bin", "llbot", "data")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !(strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "webui_token.txt" || entry.Name() == "auth_token.txt") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sourceDir, entry.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(targetDir, entry.Name()), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func luckyEntryPoint(root string) string {
	return luckyEntryPointFor(luckyPlatform(), root)
}

func luckyAuthTokenPath(root string) string {
	return filepath.Join(root, "bin", "llbot", "data", "auth_token.txt")
}

func luckyAuthTokenReady(root string) bool {
	data, err := os.ReadFile(luckyAuthTokenPath(root))
	return err == nil && strings.TrimSpace(string(data)) != ""
}

func luckySetAuthToken(params map[string]string, confirmed bool) (string, error) {
	if err := requireLuckyConfirmation(confirmed, "保存 LuckyLillia Auth Token"); err != nil {
		return "", err
	}
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if err := requireManagedLucky(state, "保存 Auth Token"); err != nil {
		return "", err
	}
	token := param(params, "authToken")
	if len(token) < 8 || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("Auth Token 无效；请粘贴从 auth.luckylillia.com 获取的完整 Token")
	}
	path := luckyAuthTokenPath(state.InstallDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := atomicPrivateText(path, token+"\n"); err != nil {
		return "", err
	}
	return "✓ Auth Token 已仅保存到本机私有文件。现在可以启动 LuckyLillia。", nil
}

// luckySetAuthTokenAndStart keeps the authorization handoff and process
// startup in one observable operation. The token itself is deliberately never
// included in progress text or logs.
func luckySetAuthTokenAndStart(params map[string]string, confirmed bool) (string, error) {
	if _, err := luckySetAuthToken(params, confirmed); err != nil {
		return "", err
	}
	reportLuckyProgress("auth", 20, "Auth Token 已安全保存到本机私有文件")
	reportLuckyProgress("start", 35, "启动 LuckyLillia 并等待管理页面")
	output, err := luckyStart(true)
	if err != nil {
		return "", err
	}
	reportLuckyProgress("complete", 100, "LuckyLillia 管理页面已就绪，等待 QQ 登录")
	return output, nil
}

func luckyEntryPointFor(platform *luckyPlatformSpec, root string) string {
	if platform == nil || root == "" || platform.Entrypoint == "" {
		return ""
	}
	path := filepath.Join(root, platform.Entrypoint)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		// Official Unix CLI packages start through `sh start.sh`. A script does
		// not need an executable bit when invoked by sh, and archive tools do not
		// reliably preserve that bit on every supported host.
		if platform.Key != "windows-amd64" && platform.Entrypoint != "start.sh" && info.Mode()&0o111 == 0 {
			return ""
		}
		return path
	}
	return ""
}

// luckyAdopt lets a user point the workbench at an already-unpacked official
// CLI package. It is intentionally held to the exact current platform entry.
func luckyAdopt(params map[string]string, confirmed bool) (string, error) {
	if err := requireLuckyConfirmation(confirmed, "关联 LuckyLillia"); err != nil {
		return "", err
	}
	dir := strings.TrimSpace(params["installDir"])
	if dir == "" || !filepath.IsAbs(dir) {
		return "", errors.New("请输入 LuckyLillia 解压目录的绝对路径")
	}
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", errors.New("指定的 LuckyLillia 安装目录不存在")
	}
	if luckyEntryPoint(dir) == "" {
		return "", errors.New("指定目录未找到 LuckyLillia 启动入口；请选择官方包解压后的根目录")
	}
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if processAlive(state.PID) && filepath.Clean(state.InstallDir) != dir {
		return "", errors.New("当前已记录的 LuckyLillia 仍在运行，请先停止后再切换安装目录")
	}
	state = luckyState{InstallDir: dir, Managed: false, InstallMode: "external", Platform: luckyPlatform().Key}
	if err := saveLuckyState(state); err != nil {
		return "", err
	}
	return "✓ 已关联现有 LuckyLillia CLI 安装目录。", nil
}

func luckyStart(confirmed bool) (string, error) {
	if err := requireLuckyConfirmation(confirmed, "启动 LuckyLillia"); err != nil {
		return "", err
	}
	if err := requireLuckySupported("启动"); err != nil {
		return "", err
	}
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if err := requireManagedLucky(state, "启动"); err != nil {
		return "", err
	}
	if !luckyAuthTokenReady(state.InstallDir) {
		return "", errors.New("缺少 LuckyLillia Auth Token；请先在“网络配置”中保存从 https://auth.luckylillia.com 获取的 Token")
	}
	if processAlive(state.PID) {
		return "? LuckyLillia 已在运行中。", nil
	}
	entry := luckyEntryPoint(state.InstallDir)
	if entry == "" {
		return "", errors.New("LuckyLillia 未安装或安装不完整，请先安装或重装")
	}
	logFile, err := luckyLogPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		return "", err
	}
	handle, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer handle.Close()
	process, err := startLuckyProcess(luckyPlatform(), state.InstallDir, entry, handle)
	if err != nil {
		return "", err
	}
	state.PID = process.PID
	state.ProcessGroupID = process.ProcessGroupID
	if err := saveLuckyState(state); err != nil {
		stopManagedProcess(state.PID)
		return "", err
	}
	webPort, _ := luckyConfiguredPorts()
	if err := waitLuckyWebUIForProcess(state.PID, webPort, webUIStartupTimeout); err != nil {
		stopManagedProcess(state.PID)
		state.PID, state.ProcessGroupID = 0, 0
		_ = saveLuckyState(state)
		return "", fmt.Errorf("LuckyLillia 启动后管理页面（端口 %d）未能就绪：%w", webPort, err)
	}
	return fmt.Sprintf("✓ LuckyLillia 已启动（PID %d）。请进入 WebUI 扫码登录。", state.PID), nil
}

func luckyStartCommand(platform *luckyPlatformSpec, root, entry string) (*exec.Cmd, error) {
	if platform == nil {
		return nil, errors.New("当前平台没有 LuckyLillia CLI 启动契约")
	}
	var command *exec.Cmd
	if platform.Entrypoint == "start.sh" {
		command = exec.Command("sh", platform.Entrypoint, "--headed", "--gui")
	} else {
		command = exec.Command(entry)
	}
	command.Dir = root
	return command, nil
}

func waitLuckyWebUI(port int, timeout time.Duration) bool {
	return waitLuckyWebUIForProcess(0, port, timeout) == nil
}

func waitLuckyWebUIForProcess(pid, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if luckyPortURL(port) != "" {
			return nil
		}
		if pid > 0 && !processAlive(pid) {
			return errors.New("LuckyLillia 进程已提前退出；请查看日志中的最后错误")
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("管理页面端口 %d 在 %s 内未监听", port, timeout.Round(time.Second))
}
func luckyStop(confirmed bool) (string, error) {
	if err := requireLuckyConfirmation(confirmed, "停止 LuckyLillia"); err != nil {
		return "", err
	}
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if err := requireManagedLucky(state, "停止"); err != nil {
		return "", err
	}
	if processAlive(state.ProcessGroupID) || processAlive(state.PID) {
		processGroupID := state.ProcessGroupID
		if processGroupID <= 0 {
			processGroupID = state.PID
		}
		stopManagedProcess(processGroupID)
		if processAlive(processGroupID) || processAlive(state.PID) {
			return "", errors.New("LuckyLillia 进程组未能停止；状态已保留以便继续诊断")
		}
	}
	state.PID, state.ProcessGroupID = 0, 0
	if err := saveLuckyState(state); err != nil {
		return "", err
	}
	return "✓ LuckyLillia 已停止。", nil
}
func luckyRestart(confirmed bool) (string, error) {
	if err := requireLuckyConfirmation(confirmed, "重启 LuckyLillia"); err != nil {
		return "", err
	}
	if err := requireLuckySupported("重启"); err != nil {
		return "", err
	}
	if _, err := luckyStop(true); err != nil {
		return "", err
	}
	return luckyStart(true)
}
func luckyUninstall(confirmed bool) (string, error) {
	if err := requireLuckyConfirmation(confirmed, "卸载 LuckyLillia"); err != nil {
		return "", err
	}
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if err := requireManagedLucky(state, "卸载"); err != nil {
		return "", err
	}
	if _, err := luckyStop(true); err != nil {
		return "", err
	}
	dir, err := luckyInstallDir()
	if err != nil {
		return "", err
	}
	if state.InstallDir != "" && filepath.Clean(state.InstallDir) != filepath.Clean(dir) {
		return "", errors.New("受管 LuckyLillia 安装目录不属于工作台；已拒绝删除")
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	return "✓ LuckyLillia 已卸载。", saveLuckyState(luckyState{})
}

func luckyForget(confirmed bool) (string, error) {
	if err := requireLuckyConfirmation(confirmed, "取消关联 LuckyLillia"); err != nil {
		return "", err
	}
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if state.Managed {
		return "", errors.New("受管安装不能取消关联；请使用卸载")
	}
	if processAlive(state.PID) {
		return "", errors.New("关联的 LuckyLillia 仍在运行，请先自行停止")
	}
	return "✓ 已取消外部 LuckyLillia CLI 关联，未删除任何文件。", saveLuckyState(luckyState{})
}
func luckyLog(params map[string]string) (string, error) {
	lines, err := linesParam(params)
	if err != nil {
		return "", err
	}
	path, err := luckyLogPath()
	if err != nil {
		return "", err
	}
	return tailLogAt(path, lines)
}
func luckyUpdateCheck() (string, error) {
	if err := requireLuckySupported("更新"); err != nil {
		return "", err
	}
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	release, err := luckyRelease()
	if err != nil {
		return "", err
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	if state.Version == "" {
		return "? LuckyLillia 尚未安装。", nil
	}
	if versionCompare(state.Version, latest) < 0 {
		return fmt.Sprintf("! 当前版本：%s\n! 最新版本：%s\n! 可执行更新。", state.Version, latest), nil
	}
	return fmt.Sprintf("✓ 当前版本：%s\n✓ 已是最新版本。", state.Version), nil
}

func luckyConfigFiles() []string {
	state, _ := loadLuckyState()
	if state.InstallDir == "" {
		state.InstallDir, _ = luckyInstallDir()
	}
	dataDir := filepath.Join(state.InstallDir, "bin", "llbot", "data")
	entries, _ := os.ReadDir(dataDir)
	files := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "config_") && strings.HasSuffix(entry.Name(), ".json") {
			files = append(files, filepath.Join(dataDir, entry.Name()))
		}
	}
	return append(files, filepath.Join(dataDir, "default_config.json"), filepath.Join(dataDir, "config.json"))
}

// luckyConfiguredPorts is deliberately shared by status and startup. A user
// may change webui.port in the official configuration; waiting on the old
// default 3080 would otherwise report a healthy instance as failed after
// three minutes and kill it.
func luckyConfiguredPorts() (webPort, oneBotPort int) {
	webPort, oneBotPort = luckyWebUIPort, luckyOneBotPort
	for _, file := range luckyConfigFiles() {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var root map[string]any
		if json.Unmarshal(data, &root) != nil {
			continue
		}
		if webui, ok := root["webui"].(map[string]any); ok {
			if port, err := strconv.Atoi(fmt.Sprint(webui["port"])); err == nil && port > 0 && port < 65536 {
				webPort = port
			}
		}
		if onebot, ok := luckyReadOneBot(root); ok && onebot.port > 0 && onebot.port < 65536 {
			oneBotPort = onebot.port
		}
		return webPort, oneBotPort
	}
	return webPort, oneBotPort
}

type luckyOneBot struct {
	enabled bool
	port    int
	token   string
}

func luckyReadOneBot(root map[string]any) (luckyOneBot, bool) {
	ob11, ok := root["ob11"].(map[string]any)
	if !ok {
		return luckyOneBot{}, false
	}
	connects, ok := ob11["connect"].([]any)
	if !ok {
		return luckyOneBot{}, false
	}
	for _, raw := range connects {
		item, ok := raw.(map[string]any)
		if !ok || fmt.Sprint(item["type"]) != "ws" {
			continue
		}
		port, _ := strconv.Atoi(fmt.Sprint(item["port"]))
		enabled, _ := item["enable"].(bool)
		return luckyOneBot{enabled: enabled, port: port, token: fmt.Sprint(item["token"])}, true
	}
	return luckyOneBot{}, false
}

func luckyOneBotConfig() (string, error) {
	for _, file := range luckyConfigFiles() {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var root map[string]any
		if json.Unmarshal(data, &root) != nil {
			continue
		}
		if config, ok := luckyReadOneBot(root); ok {
			return fmt.Sprintf("✓ LuckyLillia OneBot 配置：\n  WebSocket（启用：%v，端口：%d，Token：%s）", config.enabled, config.port, redact(config.token)), nil
		}
	}
	return "? 尚未找到 LuckyLillia OneBot 配置；保存连接配置将创建默认配置。", nil
}

func luckySetOneBotConfig(params map[string]string, confirmed bool) (string, error) {
	if err := requireLuckyConfirmation(confirmed, "保存 LuckyLillia OneBot 配置"); err != nil {
		return "", err
	}
	if err := requireLuckySupported("配置写入"); err != nil {
		return "", err
	}
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if err := requireManagedLucky(state, "写入 OneBot 配置"); err != nil {
		return "", err
	}
	port, err := portParam(params)
	if err != nil {
		return "", err
	}
	enabled, err := boolParam(params, "enable", true)
	if err != nil {
		return "", err
	}
	files := luckyConfigFiles()
	file := files[0]
	for _, candidate := range files {
		if _, err := os.Stat(candidate); err == nil {
			file = candidate
			break
		}
	}
	var root map[string]any
	if data, readErr := os.ReadFile(file); readErr == nil {
		_ = json.Unmarshal(data, &root)
	}
	if root == nil {
		root = map[string]any{}
	}
	ob11, _ := root["ob11"].(map[string]any)
	if ob11 == nil {
		ob11 = map[string]any{}
		root["ob11"] = ob11
	}
	connects, _ := ob11["connect"].([]any)
	updated := false
	for _, raw := range connects {
		item, ok := raw.(map[string]any)
		if !ok || fmt.Sprint(item["type"]) != "ws" {
			continue
		}
		item["enable"], item["port"] = enabled, port
		if token := param(params, "token"); token != "" && token != redactedToken {
			item["token"] = token
		}
		updated = true
		break
	}
	if !updated {
		connects = append(connects, map[string]any{"type": "ws", "enable": enabled, "port": port, "token": param(params, "token"), "heartInterval": 60000, "messageFormat": "array"})
	}
	ob11["enable"], ob11["connect"] = true, connects
	webui, _ := root["webui"].(map[string]any)
	if webui == nil {
		webui = map[string]any{}
		root["webui"] = webui
	}
	webui["enable"] = true
	if _, ok := webui["port"]; !ok {
		webui["port"] = luckyWebUIPort
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	if err := atomicPrivateJSON(file, append(data, '\n')); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已更新 LuckyLillia OneBot WebSocket（端口 %d）。\n✓ 重启 LuckyLillia 后生效。", port), nil
}
