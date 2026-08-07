package main

import (
	"encoding/json"
	"strings"
)

// statusPayload is the structured status returned by the `status` action so
// the web UI can poll and render it precisely. Other actions keep returning
// plain ✓/!? text.
type statusPayload struct {
	Installed      bool   `json:"installed"`
	Running        bool   `json:"running"`
	PortReachable  bool   `json:"portReachable"`
	Watchdog       bool   `json:"watchdog"`
	Version        string `json:"version,omitempty"`
	Error          string `json:"error,omitempty"`
}

// collectStatus gathers the live status from state + process probes.
func collectStatus(state State) statusPayload {
	payload := statusPayload{Version: state.Version}
	payload.Installed = state.InstallDir != "" && dirExists(state.InstallDir)
	payload.Running = processAlive(state.PID)
	payload.Watchdog = processAlive(state.WatchdogPID)
	payload.PortReachable = webUIBridge() != ""

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
	payload.Error = strings.Join(reasons, "；")
	return payload
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
