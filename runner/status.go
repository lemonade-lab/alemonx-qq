package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	// Desktop installers are intentionally download-only. Do not probe or
	// report QQ/NapCat process, WebUI, QR, OneBot, or logs: none of those are
	// owned by the workbench on macOS and Windows.
	if platform != nil && (platform.Key == "darwin-external" || platform.Key == "windows-external") {
		payload.Journey = runtimeJourney{
			Phase:      "external",
			Title:      "NapCat 本地安装器",
			Detail:     "下载完成后打开文件所在目录，并由你手动启动官方安装器。工作台不管理其进程、面板或日志。",
			NextAction: "manual",
		}
		return payload
	}
	payload.Running = isRunning(state)
	payload.Watchdog = state.Managed && napcatStateVerified(state) && processAlive(state.WatchdogPID)
	// The NapCat WebUI is separately authenticated from QQ. Its own frontend
	// accepts the configured token only at the login route (`?token=`), then
	// exchanges it for a short-lived credential kept in localStorage. The ALX
	// service proxy is same-origin, so opening its bare mount skips that official
	// bootstrap and immediately fails /api/auth/check with "Unauthorized".
	payload.WebUIURL = napcatWebUIEntryURL(state)
	payload.PortReachable = payload.WebUIURL != ""
	payload.WebUIReady = payload.PortReachable
	payload.QRCodeAvailable, payload.QRCodeUpdatedAt = napcatQRCodeStatus(state)
	// QQ 登录和 OneBot 是两段完全独立的链路。NapCat 在扫码成功回调中
	// 维护 QQLoginStatus；从其受鉴权的 WebUI API 读取这个内存态，不能
	// 用配置、端口或日志文字来猜测登录是否完成。
	payload.QQLoggedIn = napcatQQLoginStatus(state)
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

type napcatWebUIConfig struct {
	Token string `json:"token"`
}

// napcatWebUIEntryURL keeps NapCat's documented one-click WebUI login flow
// when the UI is opened through the ALX service proxy. The token is emitted
// only in the authenticated plugin status response; it is never written to an
// action result, application log, or persisted browser setting.
func napcatWebUIEntryURL(state State) string {
	if webUIBridge() == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(state.InstallDir, "config", "webui.json"))
	if err != nil {
		return ""
	}
	var config napcatWebUIConfig
	if json.Unmarshal(data, &config) != nil || strings.TrimSpace(config.Token) == "" {
		return ""
	}
	return "http://127.0.0.1:6099/webui?token=" + url.QueryEscape(config.Token)
}

type napcatAPIResponse struct {
	Code       int    `json:"code"`
	Credential string `json:"Credential"`
	Data       struct {
		Credential string `json:"Credential"`
		IsLogin    bool   `json:"isLogin"`
	} `json:"data"`
}

type napcatCredentialCache struct {
	Credential string `json:"credential"`
}

func napcatCredentialCachePath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "napcat-webui-credential.json"), nil
}

func readNapcatCredentialCache() string {
	path, err := napcatCredentialCachePath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cached napcatCredentialCache
	if json.Unmarshal(data, &cached) != nil {
		return ""
	}
	return strings.TrimSpace(cached.Credential)
}

func saveNapcatCredentialCache(credential string) {
	path, err := napcatCredentialCachePath()
	if err != nil || credential == "" {
		return
	}
	data, err := json.Marshal(napcatCredentialCache{Credential: credential})
	if err == nil {
		_ = os.WriteFile(path, data, 0o600)
	}
}

func napcatLoginStatusWithCredential(client *http.Client, credential string) (loggedIn, accepted bool) {
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:6099/api/QQLogin/CheckLoginStatus", strings.NewReader("{}"))
	if err != nil {
		return false, false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential)
	response, err := client.Do(request)
	if err != nil {
		return false, false
	}
	defer response.Body.Close()
	var status napcatAPIResponse
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&status) != nil || status.Code != 0 {
		return false, false
	}
	return status.Data.IsLogin, true
}

// napcatQQLoginStatus asks NapCat's authoritative in-memory QQLogin router.
// Its WebUI credential is cached privately and reused until rejected: status
// polling must never repeatedly invoke /auth/login, because that competes with
// an open WebUI session and made the UI flap back to its QR-login state.
func napcatQQLoginStatus(state State) bool {
	client := &http.Client{Timeout: 700 * time.Millisecond}
	if credential := readNapcatCredentialCache(); credential != "" {
		if loggedIn, accepted := napcatLoginStatusWithCredential(client, credential); accepted {
			return loggedIn
		}
	}
	configPath := filepath.Join(state.InstallDir, "config", "webui.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	var config napcatWebUIConfig
	if json.Unmarshal(data, &config) != nil || strings.TrimSpace(config.Token) == "" {
		return false
	}
	hash := sha256.Sum256([]byte(config.Token + ".napcat"))
	authBody, _ := json.Marshal(map[string]string{"hash": fmt.Sprintf("%x", hash[:])})
	authRequest, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:6099/api/auth/login", strings.NewReader(string(authBody)))
	if err != nil {
		return false
	}
	authRequest.Header.Set("Content-Type", "application/json")
	authResponse, err := client.Do(authRequest)
	if err != nil {
		return false
	}
	defer authResponse.Body.Close()
	var auth napcatAPIResponse
	if authResponse.StatusCode != http.StatusOK || json.NewDecoder(authResponse.Body).Decode(&auth) != nil {
		return false
	}
	credential := auth.Data.Credential
	if credential == "" {
		credential = auth.Credential
	}
	if credential == "" {
		return false
	}
	saveNapcatCredentialCache(credential)
	loggedIn, _ := napcatLoginStatusWithCredential(client, credential)
	return loggedIn
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
