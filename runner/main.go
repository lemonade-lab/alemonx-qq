package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const protocol = "alx/v1"

type request struct {
	Protocol string            `json:"protocol"`
	Method   string            `json:"method"`
	Action   string            `json:"action"`
	Params   map[string]string `json:"params"`
}

type response struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func main() {
	// Detached watchdog entry point for the keep-alive mode.
	if len(os.Args) >= 2 && os.Args[1] == "watchdog" {
		os.Exit(watchdogMain())
	}
	var input request
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		write(response{Error: "请求格式无效：" + err.Error()})
		return
	}
	if input.Protocol != protocol || input.Method != "run" {
		write(response{Error: fmt.Sprintf("不支持的 ALX Setup 插件协议（protocol=%q method=%q）", input.Protocol, input.Method)})
		return
	}
	output, err := run(input.Action, input.Params)
	write(response{Output: output, Error: errorText(err)})
}

func write(result response) { _ = json.NewEncoder(os.Stdout).Encode(result) }

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func run(action string, params map[string]string) (string, error) {
	switch action {
	case "status":
		return statusAction()
	case "install":
		return installAction()
	case "uninstall":
		return uninstallAction()
	case "start":
		return startAction()
	case "stop":
		return stopAction()
	case "restart":
		return restartAction()
	case "log":
		return logAction(params)
	case "onebot-config":
		cfg, err := findQQConfig()
		if err != nil {
			return "", err
		}
		return readOnebotConfig(cfg)
	case "onebot-http-set":
		return setServerConfig(params, false)
	case "onebot-ws-set":
		return setServerConfig(params, true)
	case "update-check":
		return checkUpdate()
	case "update":
		return update()
	case "watchdog-on":
		return watchdogOnAction()
	case "watchdog-off":
		return watchdogOffAction()
	default:
		return "", fmt.Errorf("未知操作：%s", action)
	}
}

func statusAction() (string, error) {
	return statusJSON()
}

func watchdogOnAction() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if processAlive(state.WatchdogPID) {
		return "? 守护模式已经在运行中。", nil
	}
	pid, err := startWatchdog()
	if err != nil {
		return "", err
	}
	state.WatchdogPID = pid
	if err := saveState(state); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 守护模式已开启（PID %d）。\n✓ NapCat 异常退出后约 15 秒会自动拉起。", pid), nil
}

func watchdogOffAction() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if !processAlive(state.WatchdogPID) {
		return "? 守护模式当前未运行。", nil
	}
	stopWatchdog(state.WatchdogPID)
	state.WatchdogPID = 0
	if err := saveState(state); err != nil {
		return "", err
	}
	return "✓ 守护模式已关闭。", nil
}

func installAction() (string, error) {
	version, err := installNapCat()
	if err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	dir, err := installDir()
	if err != nil {
		return "", err
	}
	state.Version = version
	state.InstallDir = dir
	if err := saveState(state); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ NapCat 已安装（版本 %s）。\n✓ 现在可以点击「启动」运行它。", version), nil
}

func uninstallAction() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if processAlive(state.PID) {
		stopProcess(state.PID)
	}
	state.PID = 0
	dir, err := installDir()
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	state.Version = ""
	state.InstallDir = ""
	if err := saveState(state); err != nil {
		return "", err
	}
	return "✓ NapCat 已卸载。", nil
}

func startAction() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if processAlive(state.PID) {
		return "? NapCat 已经在运行中。", nil
	}
	pid, err := startNapCat(state)
	if err != nil {
		return "", err
	}
	state.PID = pid
	if err := saveState(state); err != nil {
		return "", err
	}
	timeWait(1500)
	if url := webUIBridge(); url != "" {
		return fmt.Sprintf("✓ NapCat 已启动（PID %d）。\n✓ 管理面板可访问：%s\n✓ 用手机 QQ 扫码登录后即可使用。", pid, url), nil
	}
	return fmt.Sprintf("✓ NapCat 已启动（PID %d）。\n? 管理面板（6099）尚未就绪，请稍等片刻后查看状态。", pid), nil
}

func stopAction() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if !processAlive(state.PID) {
		return "? NapCat 当前没有在运行。", nil
	}
	stopProcess(state.PID)
	state.PID = 0
	if err := saveState(state); err != nil {
		return "", err
	}
	return "✓ NapCat 已停止。", nil
}

func restartAction() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if processAlive(state.PID) {
		stopProcess(state.PID)
		state.PID = 0
	}
	pid, err := startNapCat(state)
	if err != nil {
		return "", err
	}
	state.PID = pid
	if err := saveState(state); err != nil {
		return "", err
	}
	timeWait(1500)
	return fmt.Sprintf("✓ NapCat 已重启（PID %d）。\n%s", pid, statusLine(state)), nil
}

func logAction(params map[string]string) (string, error) {
	lines, err := linesParam(params)
	if err != nil {
		return "", err
	}
	return tailLog(lines)
}

// timeWait lets the process settle after start so the WebUI bridge probe has a
// chance to succeed before the action returns.
func timeWait(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
