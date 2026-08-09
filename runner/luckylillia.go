package main

import (
	"archive/zip"
	"crypto/sha256"
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
)

const (
	luckyReleaseURL = "https://api.github.com/repos/LLOneBot/LuckyLilliaBot/releases/latest"
	luckyAssetName  = "LLBot-CLI-linux-arm64.zip"
	luckyWebUIPort  = 3080
	luckyOneBotPort = 7199
)

// luckyReleaseValidationEvidence is injected only by the release pipeline
// after the self-hosted Linux ARM64 validation record has been reviewed.
// Empty is deliberately the safe default for every normal build.
var luckyReleaseValidationEvidence = ""

type luckyState struct {
	Version    string `json:"version,omitempty"`
	InstallDir string `json:"installDir,omitempty"`
	PID        int    `json:"pid,omitempty"`
}

type kernelStatus struct {
	Engine         string `json:"engine"`
	Installed      bool   `json:"installed"`
	InstallHealthy bool   `json:"installHealthy"`
	Running        bool   `json:"running"`
	PortReachable  bool   `json:"portReachable"`
	WebUIReady     bool   `json:"webUiReady"`
	OneBotReady    bool   `json:"oneBotReady"`
	LoginPending   bool   `json:"loginPending"`
	Version        string `json:"version,omitempty"`
	PID            int    `json:"pid,omitempty"`
	WebUIURL       string `json:"webUiUrl,omitempty"`
	OneBotURL      string `json:"oneBotUrl,omitempty"`
	LogPath        string `json:"logPath,omitempty"`
	DiagnosticHint string `json:"diagnosticHint,omitempty"`
	Error          string `json:"error,omitempty"`
	Supported      bool   `json:"supported"`
	Verified       bool   `json:"verified"`
	State          string `json:"state"`
	UpdatedAt      string `json:"updatedAt"`
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

func luckySupported() bool { return runtime.GOOS == "linux" && runtime.GOARCH == "arm64" }
func luckyVerified() bool  { return luckySupported() && luckyReleaseValidationEvidence != "" }
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
	running := processAlive(state.PID)
	webPort, oneBotPort := luckyWebUIPort, luckyOneBotPort
	for _, file := range luckyConfigFiles() {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			continue
		}
		var root map[string]any
		if json.Unmarshal(data, &root) != nil {
			continue
		}
		if webui, ok := root["webui"].(map[string]any); ok {
			if port, err := strconv.Atoi(fmt.Sprint(webui["port"])); err == nil && port > 0 {
				webPort = port
			}
		}
		if onebot, ok := luckyReadOneBot(root); ok && onebot.port > 0 {
			oneBotPort = onebot.port
		}
		break
	}
	webUI := luckyPortURL(webPort)
	onebot := luckyPortURL(oneBotPort)
	stateName := "not-installed"
	if installed {
		stateName = "stopped"
	}
	if running {
		stateName = "running"
	}
	if running && webUI != "" && onebot == "" {
		stateName = "login-pending"
	}
	if !luckySupported() {
		stateName = "unsupported"
	}
	status := kernelStatus{Engine: "luckylillia", Installed: installed, InstallHealthy: healthy, Running: running, PortReachable: webUI != "", WebUIReady: webUI != "", OneBotReady: onebot != "", LoginPending: running && webUI != "" && onebot == "", Version: state.Version, PID: state.PID, WebUIURL: webUI, OneBotURL: "ws://127.0.0.1:" + strconv.Itoa(oneBotPort), Supported: luckySupported(), Verified: luckyVerified(), State: stateName, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if path, logErr := luckyLogPath(); logErr == nil {
		status.LogPath = path
	}
	if !status.Supported {
		status.DiagnosticHint = "LuckyLillia 仅支持 Linux ARM64 自动安装。"
	} else if !status.Verified {
		status.DiagnosticHint = "LuckyLillia 实验能力尚未完成真实 Linux ARM64 验证；正式版不提供安装、更新或配置写入。"
	}
	if installed && !healthy {
		status.Error, status.DiagnosticHint = "安装目录不完整", "缺少 LuckyLillia 启动入口，请执行重装。"
	}
	if running && webUI == "" {
		status.DiagnosticHint = "进程已启动但 WebUI 未就绪，请查看日志。"
	}
	data, err := json.Marshal(status)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func luckyRelease() (githubRelease, error) {
	response, err := (&http.Client{Timeout: 20 * time.Second}).Get(luckyReleaseURL)
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

func requireLuckyVerified(action string) error {
	if !luckySupported() {
		return errors.New("LuckyLillia " + action + "仅支持 Linux ARM64")
	}
	if !luckyVerified() {
		return errors.New("LuckyLillia 实验能力尚未完成真实 Linux ARM64 验证；正式 Release 暂不允许" + action)
	}
	return nil
}

func luckyReleaseAsset(release githubRelease) (releaseAsset, error) {
	for _, asset := range release.Assets {
		if asset.Name != luckyAssetName {
			continue
		}
		digest := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:")
		if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
			return releaseAsset{}, errors.New("官方 LuckyLillia Release 未提供有效 SHA-256 校验和")
		}
		return asset, nil
	}
	return releaseAsset{}, fmt.Errorf("LuckyLillia 发布包中未找到 %s", luckyAssetName)
}

func luckyInstall(force bool) (string, error) {
	if err := requireLuckyVerified("安装"); err != nil {
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
	wasRunning := force && processAlive(previous.PID)
	restartPrevious := func() {
		if !wasRunning {
			return
		}
		_ = saveLuckyState(previous)
		_, _ = luckyStart()
	}
	if wasRunning {
		if _, err := luckyStop(); err != nil {
			return "", fmt.Errorf("停止旧 LuckyLillia 失败，未开始更新：%w", err)
		}
	}
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
	archive, err := os.CreateTemp(base, "luckylillia-*.zip")
	if err != nil {
		restartPrevious()
		return "", err
	}
	archivePath := archive.Name()
	_ = archive.Close()
	defer os.Remove(archivePath)
	if err := downloadLuckyAsset(asset, archivePath, 300<<20); err != nil {
		restartPrevious()
		return "", err
	}
	stage, err := os.MkdirTemp(base, ".luckylillia-stage-")
	if err != nil {
		restartPrevious()
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := extractLuckyZip(archivePath, stage); err != nil {
		restartPrevious()
		return "", err
	}
	root := luckyExtractRoot(stage)
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
	state := luckyState{Version: strings.TrimPrefix(release.TagName, "v"), InstallDir: target}
	if err := saveLuckyState(state); err != nil {
		_ = os.RemoveAll(target)
		if hadTarget {
			_ = os.Rename(backup, target)
		}
		restartPrevious()
		return "", err
	}
	if wasRunning {
		if _, err := luckyStart(); err != nil {
			_ = os.RemoveAll(target)
			if hadTarget {
				_ = os.Rename(backup, target)
			}
			restartPrevious()
			return "", fmt.Errorf("新版本启动失败，已回滚旧版本：%w", err)
		}
	}
	if hadTarget {
		_ = os.RemoveAll(backup)
	}
	return fmt.Sprintf("✓ LuckyLillia 已安装（版本 %s）。", state.Version), nil
}

func downloadLuckyAsset(asset releaseAsset, destination string, limit int64) error {
	if err := downloadLimited(asset.URL, destination, limit); err != nil {
		return err
	}
	file, err := os.Open(destination)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", hash.Sum(nil))
	expected := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:")
	if !strings.EqualFold(actual, expected) {
		return errors.New("LuckyLillia 安装包 SHA-256 校验失败")
	}
	return nil
}

func downloadLimited(url, destination string, limit int64) error {
	response, err := (&http.Client{Timeout: downloadTimeout}).Get(url)
	if err != nil {
		return fmt.Errorf("下载失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败（%s）", response.Status)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	count, err := io.Copy(file, io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	if count > limit {
		return errors.New("安装包超过 300 MB 限制")
	}
	return nil
}

func extractLuckyZip(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("LuckyLillia 安装包无效：%w", err)
	}
	defer reader.Close()
	var total int64
	for _, item := range reader.File {
		target := filepath.Join(destination, item.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destination)+string(filepath.Separator)) {
			return errors.New("安装包包含越界路径")
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		total += int64(item.UncompressedSize64)
		if total > 500<<20 {
			return errors.New("安装包解压后超过 500 MB 限制")
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
		if entry.IsDir() || !(strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "webui_token.txt") {
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
	if root == "" {
		return ""
	}
	for _, candidate := range []string{"llbot.js", "bin/llbot.js", "bin/llbot", "llbot"} {
		path := filepath.Join(root, candidate)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func luckyStart() (string, error) {
	if err := requireLuckyVerified("启动"); err != nil {
		return "", err
	}
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if processAlive(state.PID) {
		return "? LuckyLillia 已在运行中。", nil
	}
	entry := luckyEntryPoint(state.InstallDir)
	if entry == "" {
		return "", errors.New("LuckyLillia 未安装或安装不完整，请先安装或重装")
	}
	if err := requireNode22(); err != nil {
		return "", err
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
	var command *exec.Cmd
	if strings.HasSuffix(entry, ".js") {
		command = exec.Command("node", entry)
	} else {
		_ = os.Chmod(entry, 0o755)
		command = exec.Command(entry)
	}
	command.Dir = filepath.Dir(entry)
	command.Stdout, command.Stderr, command.Stdin = handle, handle, nil
	detachProcess(command)
	if err := command.Start(); err != nil {
		return "", err
	}
	state.PID = command.Process.Pid
	_ = command.Process.Release()
	if err := saveLuckyState(state); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ LuckyLillia 已启动（PID %d）。请进入 WebUI 扫码登录。", state.PID), nil
}
func luckyStop() (string, error) {
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	if processAlive(state.PID) {
		stopProcess(state.PID)
	}
	state.PID = 0
	if err := saveLuckyState(state); err != nil {
		return "", err
	}
	return "✓ LuckyLillia 已停止。", nil
}
func luckyRestart() (string, error) {
	if err := requireLuckyVerified("重启"); err != nil {
		return "", err
	}
	if _, err := luckyStop(); err != nil {
		return "", err
	}
	return luckyStart()
}
func luckyUninstall() (string, error) {
	if _, err := luckyStop(); err != nil {
		return "", err
	}
	dir, err := luckyInstallDir()
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	return "✓ LuckyLillia 已卸载。", saveLuckyState(luckyState{})
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
	if err := requireLuckyVerified("更新"); err != nil {
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

func luckySetOneBotConfig(params map[string]string) (string, error) {
	if err := requireLuckyVerified("配置写入"); err != nil {
		return "", err
	}
	if !luckySupported() {
		return "", errors.New("LuckyLillia 自动配置仅支持 Linux ARM64")
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
	if err := os.WriteFile(file, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已更新 LuckyLillia OneBot WebSocket（端口 %d）。\n✓ 重启 LuckyLillia 后生效。", port), nil
}

func requireNode22() error {
	output, err := exec.Command("node", "--version").Output()
	if err != nil {
		return errors.New("LuckyLillia 需要 Node.js 22 或更高版本")
	}
	version := strings.TrimSpace(strings.TrimPrefix(string(output), "v"))
	major, err := strconv.Atoi(strings.Split(version, ".")[0])
	if err != nil || major < 22 {
		return fmt.Errorf("LuckyLillia 需要 Node.js 22+，当前为 %s", version)
	}
	return nil
}
