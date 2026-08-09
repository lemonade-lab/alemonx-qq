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
	Engine          string `json:"engine"`
	Installed       bool   `json:"installed"`
	InstallHealthy  bool   `json:"installHealthy"`
	Running         bool   `json:"running"`
	PortReachable   bool   `json:"portReachable"`
	WebUIReady      bool   `json:"webUiReady"`
	OneBotReady     bool   `json:"oneBotReady"`
	LoginPending    bool   `json:"loginPending"`
	Watchdog        bool   `json:"watchdog"`
	Version         string `json:"version,omitempty"`
	PID             int    `json:"pid,omitempty"`
	WebUIURL        string `json:"webUiUrl,omitempty"`
	OneBotURL       string `json:"oneBotUrl,omitempty"`
	QRCodeAvailable bool   `json:"qrCodeAvailable"`
	QRCodeUpdatedAt string `json:"qrCodeUpdatedAt,omitempty"`
	DiagnosticHint  string `json:"diagnosticHint,omitempty"`
	Supported       bool   `json:"supported"`
	UpdatedAt       string `json:"updatedAt"`
	Error           string `json:"error,omitempty"`
}

// collectStatus gathers the live status from state + process probes.
func collectStatus(state State) statusPayload {
	payload := statusPayload{Engine: "napcat", Version: state.Version, PID: state.PID, Supported: true, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	payload.Installed = state.InstallDir != "" && dirExists(state.InstallDir)
	payload.InstallHealthy = payload.Installed
	payload.Running = isRunning(state)
	payload.Watchdog = processAlive(state.WatchdogPID)
	payload.WebUIURL = webUIBridge()
	payload.PortReachable = payload.WebUIURL != ""
	payload.WebUIReady = payload.PortReachable
	if port := napcatOneBotPort(); port > 0 {
		payload.OneBotURL = "ws://127.0.0.1:" + strconv.Itoa(port)
		connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			payload.OneBotReady = true
		}
	}
	payload.LoginPending = payload.Running && payload.WebUIReady && !payload.OneBotReady
	payload.QRCodeAvailable, payload.QRCodeUpdatedAt = napcatQRCodeStatus(state)

	reasons := []string{}
	if !payload.Installed {
		reasons = append(reasons, "未安装")
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

func napcatOneBotPort() int {
	config, err := findQQConfig()
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(config.Path)
	if err != nil {
		return 0
	}
	var document map[string]any
	if json.Unmarshal(data, &document) != nil {
		return 0
	}
	network, _ := document["network"].(map[string]any)
	servers, _ := network["websocketServers"].([]any)
	for _, raw := range servers {
		server, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		enabled, _ := server["enable"].(bool)
		if !enabled {
			continue
		}
		port, err := strconv.Atoi(fmt.Sprint(server["port"]))
		if err == nil && port > 0 {
			return port
		}
	}
	return 0
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
