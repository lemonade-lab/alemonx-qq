package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
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
		payload.DiagnosticHint = "NapCat 已启动，等待在 WebUI 中扫码登录；OneBot 服务会在登录后就绪。"
	}
	payload.Error = strings.Join(reasons, "；")
	return payload
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
