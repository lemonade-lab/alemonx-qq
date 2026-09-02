package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// statusPayload is the structured status returned by the `status` action so
// the web UI can poll and render it precisely. Other actions keep returning
// plain ✓/!? text.
type statusPayload struct {
	Engine          string          `json:"engine"`
	Installed       bool            `json:"installed"`
	InstallHealthy  bool            `json:"installHealthy"`
	Running         bool            `json:"running"`
	PortReachable   bool            `json:"portReachable"`
	WebUIReady      bool            `json:"webUiReady"`
	OneBotReady     bool            `json:"oneBotReady"`
	QQLoggedIn      bool            `json:"qqLoggedIn"`
	LoginPending    bool            `json:"loginPending"`
	Watchdog        bool            `json:"watchdog"`
	Version         string          `json:"version,omitempty"`
	PID             int             `json:"pid,omitempty"`
	WebUIURL        string          `json:"webUiUrl,omitempty"`
	OneBotURL       string          `json:"oneBotUrl,omitempty"`
	QRCodeAvailable bool            `json:"qrCodeAvailable"`
	QRCodeUpdatedAt string          `json:"qrCodeUpdatedAt,omitempty"`
	InstallerReady  bool            `json:"installerReady"`
	InstallerPath   string          `json:"installerPath,omitempty"`
	LauncherPath    string          `json:"launcherPath,omitempty"`
	LogPath         string          `json:"logPath,omitempty"`
	DiagnosticHint  string          `json:"diagnosticHint,omitempty"`
	Supported       bool            `json:"supported"`
	Managed         bool            `json:"managed"`
	Platform        string          `json:"platform,omitempty"`
	InstallMode     string          `json:"installMode,omitempty"`
	Accounts        []napcatAccount `json:"accounts,omitempty"`
	SelectedAccount string          `json:"selectedAccount,omitempty"`
	UpdatedAt       string          `json:"updatedAt"`
	Error           string          `json:"error,omitempty"`
	Journey         runtimeJourney  `json:"journey"`
}

// runtimeJourney is the user-facing runtime state machine.  It deliberately
// describes the next safe action, rather than making the browser infer a
// result from a single port probe.  Both cores expose the same contract so an
// install that is complete-but-not-authorized cannot be mistaken for a hung
// WebUI startup.
type runtimeJourney struct {
	Phase      string `json:"phase"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	NextAction string `json:"nextAction"`
}

type napcatAccount struct {
	QQ          string `json:"qq"`
	OneBotURL   string `json:"oneBotUrl,omitempty"`
	OneBotReady bool   `json:"oneBotReady"`
}

// collectStatus gathers the live status from state + process probes.
func collectStatus(state State) statusPayload {
	platform := napcatPlatform()
	payload := statusPayload{
		Engine: "napcat", Version: state.Version, PID: state.PID,
		Managed: state.Managed, InstallMode: state.InstallMode, UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if platform != nil {
		payload.Supported, payload.Platform = true, platform.Key
	}
	payload.Installed = state.InstallDir != "" && dirExists(state.InstallDir)
	if runtime.GOOS == "linux" {
		compatibility := currentLinuxHostCompatibility()
		if !compatibility.NativeSupported {
			payload.DiagnosticHint = compatibility.Diagnostic
		} else if reason := linuxSystemRuntimeDiagnostic(); reason != "" {
			payload.DiagnosticHint = "Linux 图形运行环境未就绪：" + reason + "。请先执行“准备 QQ 登录运行环境”。"
		}
	}
	if platform != nil && platform.Key == "darwin-external" && !payload.Installed && macQQInstalled() && macNapcatInjected() {
		if root, err := macInstallDir(); err == nil && dirExists(root) {
			payload.Installed, payload.InstallHealthy = true, true
		}
	}
	payload.InstallHealthy = payload.Installed
	if platform != nil && platform.Key == "darwin-external" {
		payload.InstallerReady = macInstallerReady()
		if payload.InstallerReady {
			payload.InstallerPath = macInstallerPath()
		}
		payload.LauncherPath = macNapcatLauncherPath()
	}
	if platform != nil && platform.Key == "windows-external" {
		payload.InstallerReady = windowsInstallerReady()
		if payload.InstallerReady {
			payload.InstallerPath = windowsInstallerPath()
		}
		payload.LauncherPath = windowsNapcatLauncherPath()
	}
	payload.Running = isRunning(state)
	payload.Watchdog = state.Managed && napcatStateVerified(state) && processAlive(state.WatchdogPID)
	payload.WebUIURL = webUIBridge()
	payload.PortReachable = payload.WebUIURL != ""
	payload.WebUIReady = payload.PortReachable
	payload.QRCodeAvailable, payload.QRCodeUpdatedAt = napcatQRCodeStatus(state)
	// QQ 登录和 OneBot 是两段完全独立的链路。OneBot 的配置文件会在
	// 登录前创建，端口也可能因服务重启而暂时不可用，因此两者都不能
	// 作为 QQ 登录状态的依据。NapCat 会把扫码请求与登录完成事件写入
	// 自己的进程日志；以最新事件为准，避免旧的成功记录掩盖新二维码。
	payload.QQLoggedIn = napcatQQLoggedIn(state)
	if accounts, err := napcatAccounts(state); err == nil {
		payload.Accounts = accounts
		selected := state.SelectedQQ
		if selected == "" && len(accounts) == 1 {
			selected = accounts[0].QQ
		}
		for _, account := range accounts {
			if account.QQ == selected {
				payload.SelectedAccount = account.QQ
				payload.OneBotURL = account.OneBotURL
				payload.OneBotReady = account.OneBotReady
				break
			}
		}
	}
	payload.LoginPending = payload.Running && payload.WebUIReady && !payload.QQLoggedIn
	if path, err := logPath(); err == nil {
		payload.LogPath = path
	}

	reasons := []string{}
	if !payload.Supported {
		reasons = append(reasons, "当前平台不支持 NapCat")
	} else if !payload.Installed {
		reasons = append(reasons, "未安装")
	}
	if payload.Installed && !state.Managed {
		reasons = append(reasons, "这是外部关联实例，自动操作已禁用")
	}
	if payload.Installed && !payload.Running {
		reasons = append(reasons, "进程未运行")
	}
	if payload.Installed && payload.Running && !payload.PortReachable {
		reasons = append(reasons, "管理面板（6099）不可达")
	}
	if payload.LoginPending {
		if payload.DiagnosticHint == "" {
			payload.DiagnosticHint = "NapCat 已启动，等待在 WebUI 中扫码登录；OneBot 服务会在登录后就绪。"
		}
	}
	payload.Error = strings.Join(reasons, "；")
	payload.Journey = napcatJourney(payload)
	return payload
}

// napcatQQLoggedIn reads only NapCat's own login transition log. A current
// QR request always overrides an earlier success, so a restarted instance
// correctly returns to “waiting for scan” until it records a fresh success.
// This intentionally has no dependency on OneBot configuration or ports.
func napcatQQLoggedIn(state State) bool {
	path, err := logPath()
	if err != nil {
		return false
	}
	return napcatQQLoggedInFromLog(path)
}

func napcatQQLoggedInFromLog(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return false
	}
	// The runner rotates logs at a small bounded size. Keep this guard as the
	// status call is on the UI polling path even if an old unrotated file exists.
	if len(data) > 512<<10 {
		data = data[len(data)-(512<<10):]
	}
	text := string(data)
	lastSuccess := -1
	// Do not match the generic “登录成功”: NapCat also uses that wording for
	// WebUI password authentication. These two messages are emitted only after
	// the QQ login worker has completed and notified its supervising process.
	for _, marker := range []string{"已通知主进程登录成功", "Worker进程已登录成功"} {
		if index := strings.LastIndex(text, marker); index > lastSuccess {
			lastSuccess = index
		}
	}
	lastQRCode := -1
	for _, marker := range []string{"请扫描下面的二维码", "二维码已保存到", "二维码登录方式"} {
		if index := strings.LastIndex(text, marker); index > lastQRCode {
			lastQRCode = index
		}
	}
	return lastSuccess >= 0 && lastSuccess > lastQRCode
}

func napcatJourney(status statusPayload) runtimeJourney {
	switch {
	case !status.Supported:
		return runtimeJourney{Phase: "unsupported", Title: "当前系统暂不支持", Detail: firstStatusDetail(status.DiagnosticHint, status.Error, "请使用受支持的系统或手动部署 NapCat。"), NextAction: "manual"}
	case !status.Installed:
		return runtimeJourney{Phase: "install", Title: "安装 NapCat", Detail: "将先验证运行环境，再下载、安装并启动 QQ 登录服务。", NextAction: "install"}
	case !status.InstallHealthy:
		return runtimeJourney{Phase: "repair", Title: "NapCat 安装不完整", Detail: firstStatusDetail(status.DiagnosticHint, "请重新安装后再启动。"), NextAction: "repair"}
	case !status.Managed:
		return runtimeJourney{Phase: "external", Title: "NapCat 已关联", Detail: firstStatusDetail(status.DiagnosticHint, "这是外部实例；工作台不会修改其进程或配置。"), NextAction: "open-webui"}
	case !status.Running:
		return runtimeJourney{Phase: "start", Title: "启动 NapCat", Detail: "启动后将等待 QQ 登录二维码和 OneBot 服务就绪。", NextAction: "start"}
	case !status.WebUIReady:
		return runtimeJourney{Phase: "starting", Title: "正在启动 NapCat", Detail: firstStatusDetail(status.DiagnosticHint, "进程已启动，正在等待管理面板（6099）就绪。"), NextAction: "view-log"}
	case status.LoginPending:
		return runtimeJourney{Phase: "scan-qq", Title: "请用手机 QQ 扫码", Detail: "登录成功后会自动继续初始化 OneBot 服务。", NextAction: "scan-qq"}
	case !status.OneBotReady:
		return runtimeJourney{Phase: "connecting", Title: "QQ 已登录，正在等待 OneBot", Detail: "QQ 登录已确认；请启用并重启已配置的 OneBot 服务，使其开始监听端口。", NextAction: "view-log"}
	default:
		return runtimeJourney{Phase: "ready", Title: "NapCat 已就绪", Detail: "QQ 与 OneBot 服务均已可用，可同步到机器人。", NextAction: "configure"}
	}
}

func firstStatusDetail(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func napcatAccounts(state State) ([]napcatAccount, error) {
	configs, err := findQQConfigs(state)
	if err != nil {
		return nil, err
	}
	items := make([]napcatAccount, 0, len(configs))
	for _, config := range configs {
		data, readErr := os.ReadFile(config.Path)
		if readErr != nil {
			continue
		}
		var document map[string]any
		if json.Unmarshal(data, &document) != nil {
			continue
		}
		network, _ := document["network"].(map[string]any)
		servers, _ := network["websocketServers"].([]any)
		item := napcatAccount{QQ: config.QQ}
		for _, raw := range servers {
			server, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			enabled, _ := server["enable"].(bool)
			if !enabled {
				continue
			}
			port, portErr := strconv.Atoi(fmt.Sprint(server["port"]))
			if portErr != nil || port <= 0 {
				continue
			}
			item.OneBotURL = "ws://127.0.0.1:" + strconv.Itoa(port)
			connection, dialErr := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
			if dialErr == nil {
				_ = connection.Close()
				item.OneBotReady = true
			}
			break
		}
		items = append(items, item)
	}
	return items, nil
}

// statusJSON returns the structured status as a JSON string.
func statusJSON() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	payload := collectStatus(state)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
