package main

import (
	"archive/zip"
	"compress/gzip"
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

const snowLumaReleaseURL = "https://api.github.com/repos/SnowLuma/SnowLuma/releases/latest"

type snowLumaState struct {
	Version, InstallDir, Asset string
	PID, ProcessGroupID        int
	Managed                    bool
}

type snowLumaPorts struct {
	WebUI, OneBot int
	OneBotEnabled bool
}

func snowLumaPlatform() (asset, native string, ok bool) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return "win-x64.zip", "snowluma-win32-x64.node", true
	case "linux/amd64":
		return "linux-x64.tar.gz", "snowluma-linux-x64.node", true
	case "linux/arm64":
		return "linux-arm64.tar.gz", "snowluma-linux-arm64.node", true
	default:
		return "", "", false
	}
}
func snowLumaStatePath() (string, error) {
	d, e := stateDir()
	return filepath.Join(d, "snowluma-state.json"), e
}
func snowLumaInstallDir() (string, error) { d, e := stateDir(); return filepath.Join(d, "snowluma"), e }
func snowLumaLogPath() (string, error) {
	d, e := stateDir()
	return filepath.Join(d, "snowluma.log"), e
}
func snowLumaOperationLogPath() (string, error) {
	d, e := stateDir()
	return filepath.Join(d, "snowluma-operation.log"), e
}
func loadSnowLumaState() (snowLumaState, error) {
	p, e := snowLumaStatePath()
	if e != nil {
		return snowLumaState{}, e
	}
	b, e := os.ReadFile(p)
	if errors.Is(e, os.ErrNotExist) {
		return snowLumaState{}, nil
	}
	var s snowLumaState
	if e == nil {
		e = json.Unmarshal(b, &s)
	}
	return s, e
}
func saveSnowLumaState(s snowLumaState) error {
	p, e := snowLumaStatePath()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	return atomicPrivateText(p, string(b)+"\n")
}
func snowLumaSupported() bool { _, _, ok := snowLumaPlatform(); return ok }
func reportSnowLumaProgress(stage string, p int, msg string) {
	appendActionDiagnostic(currentSnowLumaOperationAction(), fmt.Sprintf("[%s] %d%% %s", time.Now().UTC().Format(time.RFC3339), p, msg))
	b, _ := json.Marshal(map[string]any{"stage": stage, "percent": p, "message": msg})
	_, _ = fmt.Fprintf(os.Stderr, "@alx-progress %s\n", b)
}
func currentSnowLumaOperationAction() string {
	if a := currentOperationAction(); strings.HasPrefix(a, "snowluma-") {
		return a
	}
	return "snowluma-install"
}

func snowLumaRelease() (githubRelease, error) { return fetchRelease(snowLumaReleaseURL, "SnowLuma") }
func snowLumaAsset(r githubRelease) (releaseAsset, error) {
	suffix, _, ok := snowLumaPlatform()
	if !ok {
		return releaseAsset{}, errors.New("SnowLuma 原生 Hook 当前仅支持 Windows x64、Linux x64、Linux ARM64；macOS 尚无官方 Darwin Hook")
	}
	for _, a := range r.Assets {
		if strings.HasSuffix(a.Name, suffix) && !strings.Contains(a.Name, "-lite.") {
			return a, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("官方发布包中未找到 SnowLuma %s 完整包", suffix)
}
func snowLumaRoot(dir string) string {
	es, e := os.ReadDir(dir)
	if e == nil && len(es) == 1 && es[0].IsDir() {
		return filepath.Join(dir, es[0].Name())
	}
	return dir
}
func snowLumaEntry(root string) string {
	p := filepath.Join(root, "index.mjs")
	if info, e := os.Stat(p); e == nil && !info.IsDir() {
		return p
	}
	return ""
}
func snowLumaNativeReady(root string) bool {
	_, n, ok := snowLumaPlatform()
	if !ok {
		return false
	}
	for _, d := range []string{"native", "dist/native", "packages/runtime/native"} {
		if f, e := os.Stat(filepath.Join(root, d, n)); e == nil && !f.IsDir() {
			return true
		}
	}
	return false
}
func extractSnowLuma(a, d string) error {
	if strings.HasSuffix(a, ".zip") {
		r, e := zip.OpenReader(a)
		if e != nil {
			return e
		}
		defer r.Close()
		for _, f := range r.File {
			t, e := secureArchiveTarget(d, f.Name)
			if e != nil {
				return e
			}
			if f.FileInfo().IsDir() {
				if e = os.MkdirAll(t, 0755); e != nil {
					return e
				}
				continue
			}
			if f.Mode()&os.ModeSymlink != 0 {
				return errors.New("SnowLuma 安装包不能包含符号链接")
			}
			if e = os.MkdirAll(filepath.Dir(t), 0755); e != nil {
				return e
			}
			in, e := f.Open()
			if e != nil {
				return e
			}
			out, e := os.OpenFile(t, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode()|0600)
			if e == nil {
				_, e = io.Copy(out, in)
				_ = out.Close()
			}
			_ = in.Close()
			if e != nil {
				return e
			}
		}
		return nil
	}
	in, e := os.Open(a)
	if e != nil {
		return e
	}
	defer in.Close()
	gz, e := gzip.NewReader(in)
	if e != nil {
		return e
	}
	defer gz.Close()
	return extractLinuxTar(gz, d)
}

func requireSnowLumaConfirmation(c bool, a string) error {
	if !c {
		return errors.New("请确认后再" + a)
	}
	return nil
}
func snowLumaInstall(_ map[string]string, c bool) (string, error) {
	if e := requireSnowLumaConfirmation(c, "安装 SnowLuma"); e != nil {
		return "", e
	}
	return snowLumaInstallRelease(false)
}

// snowLumaInstallRelease replaces only the program directory. SnowLuma keeps
// login state and OneBot configuration in its data directory, which must never
// be deleted as part of an upgrade.
func snowLumaInstallRelease(replace bool) (string, error) {
	if !snowLumaSupported() {
		return "", errors.New("SnowLuma 原生 Hook 当前仅支持 Windows x64、Linux x64、Linux ARM64；macOS 尚无官方 Darwin Hook")
	}
	s, e := loadSnowLumaState()
	if e != nil {
		return "", e
	}
	if !replace && s.Managed && snowLumaEntry(s.InstallDir) != "" {
		return "? SnowLuma 已安装。", nil
	}
	reportSnowLumaProgress("prepare", 5, "准备 SnowLuma 原生完整发行包")
	r, e := snowLumaRelease()
	if e != nil {
		return "", e
	}
	a, e := snowLumaAsset(r)
	if e != nil {
		return "", e
	}
	base, e := stateDir()
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(base, 0755); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(base, "snowluma-*")
	if e != nil {
		return "", e
	}
	archive := f.Name()
	_ = f.Close()
	defer os.Remove(archive)
	reportSnowLumaProgress("download", 20, "下载官方 SnowLuma 完整包")
	if e = downloadFileWithProgress(a.URL, archive, napcatDownloadProgress("下载官方 SnowLuma 完整包", 20, 55)); e != nil {
		return "", e
	}
	stage, e := os.MkdirTemp(base, ".snowluma-stage-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(stage)
	reportSnowLumaProgress("extract", 60, "安全解压并验证原生 Hook")
	if e = extractSnowLuma(archive, stage); e != nil {
		return "", e
	}
	root := snowLumaRoot(stage)
	if snowLumaEntry(root) == "" || !snowLumaNativeReady(root) {
		return "", errors.New("SnowLuma 完整包缺少 index.mjs 或当前架构原生 Hook")
	}
	target, e := snowLumaInstallDir()
	if e != nil {
		return "", e
	}
	backup := target + ".previous"
	if _, e = os.Stat(backup); e == nil {
		return "", errors.New("检测到未清理的 SnowLuma 备份目录")
	}
	if _, e = os.Stat(target); e == nil {
		if e = os.Rename(target, backup); e != nil {
			return "", e
		}
	}
	if e = os.Rename(root, target); e != nil {
		_ = os.Rename(backup, target)
		return "", e
	}
	if _, e = os.Stat(backup); e == nil {
		_ = os.RemoveAll(backup)
	}
	s = snowLumaState{Version: strings.TrimPrefix(r.TagName, "v"), InstallDir: target, Asset: a.Name, Managed: true}
	if e = saveSnowLumaState(s); e != nil {
		// The new directory is valid but state persistence failed. Restore the
		// previous program directory so the next retry has a known-good base.
		_ = os.RemoveAll(target)
		if _, backupErr := os.Stat(backup); backupErr == nil {
			_ = os.Rename(backup, target)
		}
		return "", e
	}
	reportSnowLumaProgress("complete", 100, "SnowLuma 原生内核安装完成")
	return "✓ SnowLuma 原生完整包已安装。请启动本机 QQ 后点击启动。", nil
}

func snowLumaCommand(root string) (*exec.Cmd, error) {
	entry := snowLumaEntry(root)
	if entry == "" {
		return nil, errors.New("SnowLuma 安装不完整")
	}
	if runtime.GOOS == "windows" {
		for _, p := range []string{filepath.Join(root, "node.exe"), filepath.Join(root, "bin", "node.exe")} {
			if info, e := os.Stat(p); e == nil && !info.IsDir() {
				return exec.Command(p, entry), nil
			}
		}
		node, e := exec.LookPath("node.exe")
		if e != nil {
			return nil, errors.New("完整包未找到内置 Node，且系统未安装 node.exe")
		}
		return exec.Command(node, entry), nil
	}
	for _, p := range []string{filepath.Join(root, "node"), filepath.Join(root, "bin", "node")} {
		if _, e := os.Stat(p); e == nil {
			return exec.Command(p, entry), nil
		}
	}
	node, e := exec.LookPath("node")
	if e != nil {
		return nil, errors.New("完整包未找到内置 Node，且系统未安装 node")
	}
	return exec.Command(node, entry), nil
}

func snowLumaQQRunning() bool {
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		command = exec.Command("tasklist", "/FO", "CSV", "/NH")
	} else {
		command = exec.Command("ps", "-A", "-o", "comm=")
	}
	output, err := command.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.ToLower(string(output)), "\n") {
		name := strings.TrimSpace(line)
		if runtime.GOOS == "windows" {
			name = strings.Trim(strings.SplitN(name, ",", 2)[0], "\"")
		}
		if name == "qq" || name == "qq.exe" || strings.HasSuffix(name, "/qq") || strings.HasSuffix(name, "\\qq.exe") {
			return true
		}
	}
	return false
}

func snowLumaLinuxReady() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if strings.TrimSpace(os.Getenv("DISPLAY")) == "" {
		return errors.New("Linux 未设置 DISPLAY；请先在同一用户的 X11/Xvfb 图形会话中启动 QQ，再启动 SnowLuma")
	}
	// A non-parent process cannot attach under Yama restricted mode without
	// CAP_SYS_PTRACE. Refuse early instead of claiming that QR login is ready.
	if data, err := os.ReadFile("/proc/sys/kernel/yama/ptrace_scope"); err == nil {
		if level, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil && level > 0 {
			return fmt.Errorf("Linux ptrace_scope=%d 会阻止 SnowLuma 注入 QQ；请按 SnowLuma 原生部署要求为运行 Node 授予 CAP_SYS_PTRACE，或调整此主机策略", level)
		}
	}
	return nil
}

func snowLumaPreflight(root string) error {
	if snowLumaEntry(root) == "" || !snowLumaNativeReady(root) {
		return errors.New("SnowLuma 安装不完整，请重新安装")
	}
	if !snowLumaQQRunning() {
		return errors.New("未检测到同一系统用户下的 QQ 进程；请先启动 QQ，看到登录二维码后再启动 SnowLuma")
	}
	return snowLumaLinuxReady()
}

func snowLumaConfigPaths(root string) []string {
	return []string{
		filepath.Join(root, "config", "onebot.json"),
		filepath.Join(root, "data", "config", "onebot.json"),
		filepath.Join(root, "snowluma-data", "config", "onebot.json"),
	}
}

func snowLumaRuntimePaths(root string) []string {
	return []string{
		filepath.Join(root, "config", "runtime.json"),
		filepath.Join(root, "data", "config", "runtime.json"),
		filepath.Join(root, "snowluma-data", "config", "runtime.json"),
	}
}

func snowLumaPortsFor(root string) snowLumaPorts {
	ports := snowLumaPorts{WebUI: 5099, OneBot: 3001, OneBotEnabled: true}
	for _, path := range snowLumaRuntimePaths(root) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(data, &doc) != nil {
			continue
		}
		if port := intValue(doc["webuiPort"]); port > 0 && port < 65536 {
			ports.WebUI = port
		}
		break
	}
	for _, path := range snowLumaConfigPaths(root) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(data, &doc) != nil {
			continue
		}
		networks, _ := readMap(doc["networks"])
		servers, _ := networks["wsServers"].([]any)
		if len(servers) == 0 {
			servers, _ = doc["wsServers"].([]any)
		}
		if len(servers) > 0 {
			ports.OneBotEnabled = false
			for _, raw := range servers {
				server, ok := readMap(raw)
				if !ok || server["enabled"] == false {
					continue
				}
				if port := intValue(server["port"]); port > 0 && port < 65536 {
					ports.OneBot = port
				}
				ports.OneBotEnabled = true
				break
			}
		}
		break
	}
	return ports
}

func readMap(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}
func intValue(value any) int {
	switch value := value.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		n, _ := strconv.Atoi(value)
		return n
	default:
		return 0
	}
}

func snowLumaPortOpen(port int) bool {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 350*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func snowLumaWebUIURL(port int) string {
	client := &http.Client{Timeout: 900 * time.Millisecond}
	response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/")
	if err != nil {
		return ""
	}
	_ = response.Body.Close()
	if response.StatusCode >= 500 {
		return ""
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}
func snowLumaStart(c bool) (string, error) {
	if e := requireSnowLumaConfirmation(c, "启动 SnowLuma"); e != nil {
		return "", e
	}
	s, e := loadSnowLumaState()
	if e != nil {
		return "", e
	}
	if !s.Managed || s.InstallDir == "" {
		return "", errors.New("请先安装 SnowLuma")
	}
	if processAlive(s.PID) {
		return "? SnowLuma 已在运行。", nil
	}
	if e = snowLumaPreflight(s.InstallDir); e != nil {
		return "", e
	}
	ports := snowLumaPortsFor(s.InstallDir)
	if snowLumaPortOpen(ports.WebUI) {
		return "", fmt.Errorf("SnowLuma WebUI 端口 %d 已被占用；为避免误判，未启动新的 SnowLuma 进程", ports.WebUI)
	}
	cmd, e := snowLumaCommand(s.InstallDir)
	if e != nil {
		return "", e
	}
	log, e := snowLumaLogPath()
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(filepath.Dir(log), 0755); e != nil {
		return "", e
	}
	h, e := openAppendLog(log)
	if e != nil {
		return "", e
	}
	defer h.Close()
	cmd.Dir = s.InstallDir
	cmd.Env = append(os.Environ(), "SNOWLUMA_HOOK_AUTOLOAD=1")
	cmd.Stdout, cmd.Stderr, cmd.Stdin = h, h, nil
	detachProcess(cmd)
	reportSnowLumaProgress("start", 60, "启动 SnowLuma 原生进程并等待 WebUI")
	if e = cmd.Start(); e != nil {
		return "", e
	}
	s.PID = safePID(cmd)
	s.ProcessGroupID = s.PID
	_ = cmd.Process.Release()
	if e = saveSnowLumaState(s); e != nil {
		return "", e
	}
	if e = waitSnowLumaWebUI(s.PID, ports.WebUI, 45*time.Second); e != nil {
		return "", e
	}
	reportSnowLumaProgress("complete", 100, "SnowLuma WebUI 已就绪，等待 QQ 登录")
	return "✓ SnowLuma 已启动。请使用同一系统用户运行本机 QQ 并扫码登录。", nil
}
func safePID(c *exec.Cmd) int {
	if c != nil && c.Process != nil {
		return c.Process.Pid
	}
	return 0
}
func waitSnowLumaWebUI(pid, port int, t time.Duration) error {
	d := time.Now().Add(t)
	for time.Now().Before(d) {
		if snowLumaWebUIURL(port) != "" {
			return nil
		}
		if pid > 0 && !processAlive(pid) {
			return errors.New("SnowLuma 进程提前退出，请查看日志")
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("SnowLuma WebUI（%d）未就绪，请查看日志", port)
}
func snowLumaStop(c bool) (string, error) {
	if e := requireSnowLumaConfirmation(c, "停止 SnowLuma"); e != nil {
		return "", e
	}
	s, e := loadSnowLumaState()
	if e != nil {
		return "", e
	}
	if s.PID > 0 {
		stopManagedProcess(s.ProcessGroupID)
	}
	s.PID, s.ProcessGroupID = 0, 0
	return "✓ SnowLuma 已停止。", saveSnowLumaState(s)
}
func snowLumaRestart(c bool) (string, error) {
	if _, e := snowLumaStop(c); e != nil {
		return "", e
	}
	return snowLumaStart(true)
}
func snowLumaUpdate(c bool) (string, error) {
	if err := requireSnowLumaConfirmation(c, "更新 SnowLuma"); err != nil {
		return "", err
	}
	s, err := loadSnowLumaState()
	if err != nil {
		return "", err
	}
	if !s.Managed || s.InstallDir == "" {
		return "", errors.New("请先安装 SnowLuma")
	}
	wasRunning := processAlive(s.PID)
	if wasRunning {
		reportSnowLumaProgress("stop", 10, "停止旧版 SnowLuma 进程")
		if _, err = snowLumaStop(true); err != nil {
			return "", err
		}
	}
	reportSnowLumaProgress("update", 20, "下载并原子替换 SnowLuma 程序包（保留数据目录）")
	output, err := snowLumaInstallRelease(true)
	if err != nil {
		return "", err
	}
	if wasRunning {
		reportSnowLumaProgress("start", 85, "恢复 SnowLuma 进程")
		return snowLumaStart(true)
	}
	return output, nil
}
func snowLumaUninstall(c bool) (string, error) {
	if e := requireSnowLumaConfirmation(c, "卸载 SnowLuma"); e != nil {
		return "", e
	}
	s, e := loadSnowLumaState()
	if e != nil {
		return "", e
	}
	if _, e = snowLumaStop(true); e != nil {
		return "", e
	}
	target, _ := snowLumaInstallDir()
	if filepath.Clean(s.InstallDir) != filepath.Clean(target) {
		return "", errors.New("已拒绝删除非受管目录")
	}
	if e = os.RemoveAll(target); e != nil {
		return "", e
	}
	return "✓ SnowLuma 已卸载。", saveSnowLumaState(snowLumaState{})
}
func snowLumaLog(p map[string]string) (string, error) {
	n, e := linesParam(p)
	if e != nil {
		return "", e
	}
	f, e := snowLumaLogPath()
	if e != nil {
		return "", e
	}
	return tailLogAt(f, n)
}
func snowLumaStatus() (string, error) {
	s, e := loadSnowLumaState()
	if e != nil {
		return "", e
	}
	installed := s.InstallDir != "" && snowLumaEntry(s.InstallDir) != "" && snowLumaNativeReady(s.InstallDir)
	running := processAlive(s.PID)
	ports := snowLumaPortsFor(s.InstallDir)
	web := ""
	if running {
		web = snowLumaWebUIURL(ports.WebUI)
	}
	_, _, ok := snowLumaPlatform()
	oneBotReady := running && ports.OneBotEnabled && snowLumaPortOpen(ports.OneBot)
	x := kernelStatus{Engine: "snowluma", Installed: installed, InstallHealthy: installed, Running: running, PortReachable: web != "", WebUIReady: web != "", OneBotReady: oneBotReady, LoginPending: running && web != "" && !oneBotReady, Version: s.Version, PID: s.PID, WebUIURL: web, OneBotURL: "ws://127.0.0.1:" + strconv.Itoa(ports.OneBot), Supported: ok, Managed: s.Managed, State: "stopped", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if !ok {
		x.State = "unsupported"
		x.DiagnosticHint = "上游未发布 macOS/Darwin Hook，无法注入 QQ 进程。"
	} else if !installed {
		x.State = "not-installed"
	} else if running {
		x.State = "running"
	}
	x.Journey = snowLumaJourney(x)
	b, e := json.Marshal(x)
	return string(b), e
}
func snowLumaJourney(s kernelStatus) runtimeJourney {
	if !s.Supported {
		return runtimeJourney{Phase: "unsupported", Title: "当前系统无原生 Hook", Detail: s.DiagnosticHint, NextAction: "manual"}
	}
	if !s.Installed {
		return runtimeJourney{Phase: "install", Title: "安装 SnowLuma", Detail: "下载官方完整包，不使用 Docker。", NextAction: "install"}
	}
	if !s.Running {
		return runtimeJourney{Phase: "start", Title: "启动 SnowLuma", Detail: "启动后使用同一用户运行本机 QQ。", NextAction: "start"}
	}
	if !s.WebUIReady {
		return runtimeJourney{Phase: "starting", Title: "正在启动 SnowLuma", Detail: "等待 WebUI（5099）就绪。", NextAction: "view-log"}
	}
	if s.LoginPending {
		return runtimeJourney{Phase: "scan-qq", Title: "请在本机 QQ 扫码", Detail: "SnowLuma 会自动发现并注入本机 QQ 进程。", NextAction: "scan-qq"}
	}
	return runtimeJourney{Phase: "ready", Title: "SnowLuma 已就绪", Detail: "OneBot WebSocket 已监听。", NextAction: "configure"}
}
func snowLumaOperationStatusAction() (string, error) {
	p, e := snowLumaOperationLogPath()
	return operationStatusAt(p, e)
}
func snowLumaLogStatusAction() (string, error) {
	o, e := snowLumaLog(nil)
	b, _ := json.Marshal(map[string]string{"output": o})
	return string(b), e
}
func clearSnowLumaLogs() (string, error) {
	p, e := snowLumaOperationLogPath()
	if e != nil {
		return "", e
	}
	return "✓ SnowLuma 操作日志已清空。", clearLogAt(p)
}
func snowLumaOneBotToken() (string, error) {
	s, err := loadSnowLumaState()
	if err != nil {
		return "", err
	}
	if !s.Managed || s.InstallDir == "" {
		return "", errors.New("请先安装 SnowLuma")
	}
	for _, path := range snowLumaConfigPaths(s.InstallDir) {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(data, &doc) != nil {
			continue
		}
		networks, _ := readMap(doc["networks"])
		servers, _ := networks["wsServers"].([]any)
		if len(servers) == 0 {
			servers, _ = doc["wsServers"].([]any)
		}
		for _, raw := range servers {
			server, ok := readMap(raw)
			if !ok || server["enabled"] == false {
				continue
			}
			token := strings.TrimSpace(fmt.Sprint(server["accessToken"]))
			if token != "" && token != "<nil>" {
				return oneBotTokenPayload(token)
			}
		}
	}
	return "", errors.New("未找到 SnowLuma WebSocket Token；请先在 WebUI 完成 QQ 登录并启用 OneBot WebSocket")
}
