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
	payload.LoginPending = payload.Running && payload.WebUIReady && !payload.OneBotReady
	payload.QRCodeAvailable, payload.QRCodeUpdatedAt = napcatQRCodeStatus(state)
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
		return runtimeJourney{Phase: "connecting", Title: "正在等待 OneBot", Detail: "QQ 已登录，正在等待已配置的 OneBot 服务监听端口。", NextAction: "view-log"}
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
