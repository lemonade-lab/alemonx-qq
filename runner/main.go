package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const protocol = "alx/v1"

type request struct {
	Protocol string            `json:"protocol"`
	Method   string            `json:"method"`
	Action   string            `json:"action"`
	Params   map[string]string `json:"params"`
	Confirm  bool              `json:"confirm"`
}

type response struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func main() {
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
	if napcatOperationAction(input.Action) {
		resetActionDiagnostic("install")
		appendActionDiagnostic("install", fmt.Sprintf("[%s] action=%s started", time.Now().UTC().Format(time.RFC3339), input.Action))
	}
	output, err := run(input.Action, input.Params, input.Confirm)
	if err != nil {
		recordActionFailure(input.Action, err)
	} else if napcatOperationAction(input.Action) {
		appendActionDiagnostic("install", fmt.Sprintf("[%s] action=%s completed\n%s", time.Now().UTC().Format(time.RFC3339), input.Action, output))
	}
	write(response{Output: output, Error: errorText(err)})
}

func napcatOperationAction(action string) bool {
	switch action {
	case "install", "update", "start", "restart":
		return true
	default:
		return false
	}
}

func write(result response) { _ = json.NewEncoder(os.Stdout).Encode(result) }
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func run(action string, params map[string]string, confirmed bool) (string, error) {
	if napcatLifecycleAction(action) {
		return withNapcatLifecycleLock(func() (string, error) {
			return runNapcatAction(action, params, confirmed)
		})
	}
	return runNapcatAction(action, params, confirmed)
}

func napcatLifecycleAction(action string) bool {
	switch action {
	case "install", "uninstall", "start", "stop", "restart", "update", "watchdog-on", "watchdog-off":
		return true
	default:
		return false
	}
}

func runNapcatAction(action string, params map[string]string, confirmed bool) (string, error) {
	switch action {
	case "status", "napcat-status":
		return statusAction()
	case "napcat-qrcode":
		return napcatQRCodeAction()
	case "napcat-adopt":
		return napcatAdopt(params, confirmed)
	case "napcat-forget":
		return napcatForget(confirmed)
	case "napcat-macos-installer-download":
		if err := requireNapcatConfirmation(confirmed, "下载 macOS NapCat 安装器"); err != nil {
			return "", err
		}
		return downloadMacNapcatInstaller()
	case "napcat-macos-installer-open":
		if err := requireNapcatConfirmation(confirmed, "打开 macOS NapCat 安装器"); err != nil {
			return "", err
		}
		return openMacNapcatInstaller()
	case "napcat-macos-launcher-open":
		if err := requireNapcatConfirmation(confirmed, "打开 NapCat 启动器"); err != nil {
			return "", err
		}
		return openMacNapcatLauncher()
	case "napcat-windows-installer-download":
		if err := requireNapcatConfirmation(confirmed, "下载 Windows NapCat 安装器"); err != nil {
			return "", err
		}
		return downloadWindowsNapcatInstaller()
	case "napcat-windows-installer-open", "napcat-windows-launcher-open":
		if err := requireNapcatConfirmation(confirmed, "打开 NapCat 启动器"); err != nil {
			return "", err
		}
		return openWindowsNapcatLauncher()
	case "install":
		return installAction(params, confirmed)
	case "uninstall":
		return uninstallAction(confirmed)
	case "start":
		return startAction(confirmed)
	case "stop":
		return stopAction(confirmed)
	case "restart":
		return restartAction(confirmed)
	case "log":
		return logAction(params)
	case "napcat-log-status":
		return logStatusAction(false)
	case "napcat-operation-status":
		return napcatOperationStatusAction()
	case "onebot-config":
		return onebotConfigAction(params)
	case "onebot-http-set":
		return setServerConfig(params, false, confirmed)
	case "onebot-ws-set":
		return setServerConfig(params, true, confirmed)
	case "update-check":
		return checkUpdate()
	case "update":
		return updateNapcat(confirmed)
	case "watchdog-on":
		return watchdogOnAction(confirmed)
	case "watchdog-off":
		return watchdogOffAction(confirmed)
	case "luckylillia-status":
		return luckyStatus()
	case "luckylillia-install":
		return luckyInstall(false, confirmed)
	case "luckylillia-adopt":
		return luckyAdopt(params, confirmed)
	case "luckylillia-reinstall":
		return luckyInstall(true, confirmed)
	case "luckylillia-start":
		return luckyStart(confirmed)
	case "luckylillia-stop":
		return luckyStop(confirmed)
	case "luckylillia-restart":
		return luckyRestart(confirmed)
	case "luckylillia-uninstall":
		return luckyUninstall(confirmed)
	case "luckylillia-forget":
		return luckyForget(confirmed)
	case "luckylillia-update-check":
		return luckyUpdateCheck()
	case "luckylillia-update":
		return luckyInstall(true, confirmed)
	case "luckylillia-log":
		return luckyLog(params)
	case "luckylillia-log-status":
		return logStatusAction(true)
	case "luckylillia-onebot-config":
		return luckyOneBotConfig()
	case "luckylillia-onebot-set":
		return luckySetOneBotConfig(params, confirmed)
	default:
		return "", fmt.Errorf("未知操作：%s", action)
	}
}

func napcatOperationStatusAction() (string, error) {
	path, err := napcatOperationLogPath()
	if err != nil {
		return "", err
	}
	output, err := tailLogAt(path, 500)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]string{"output": output})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func statusAction() (string, error) { return statusJSON() }

func logStatusAction(lucky bool) (string, error) {
	var (
		output string
		err    error
	)
	if lucky {
		output, err = luckyLog(nil)
	} else {
		output, err = logAction(nil)
	}
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]string{"output": output})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func napcatAdopt(params map[string]string, confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "关联外部 NapCat"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if state.Managed {
		return "", errors.New("受管 NapCat 不能关联为外部实例；请先卸载")
	}
	dir := param(params, "installDir")
	if dir == "" && runtime.GOOS == "darwin" {
		dir, err = platformInstallDir()
		if err != nil {
			return "", err
		}
	}
	if !filepath.IsAbs(dir) || !dirExists(dir) {
		return "", errors.New("请输入已安装 NapCat 的绝对目录")
	}
	if runtime.GOOS == "darwin" {
		expected, expectedErr := macInstallDir()
		if expectedErr != nil || !macQQInstalled() || !macNapcatInjected() || filepath.Clean(dir) != filepath.Clean(expected) {
			return "", errors.New("未检测到可关联的 QQ 注入式 NapCat；macOS 仅允许关联 QQ 容器中的现有实例")
		}
	}
	platform := napcatPlatform()
	label := "当前平台"
	platformKey := "external"
	if platform != nil {
		label = platform.Label
		platformKey = platform.Key
	}
	state = State{InstallDir: dir, InstallMode: "external", Managed: false, Platform: platformKey}
	if runtime.GOOS == "darwin" {
		state.Version = macNapcatVersion()
	}
	if err := saveState(state); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已关联 %s 的外部 NapCat；工作台仅提供状态、二维码与 WebUI，不会修改该目录。", label), nil
}

func napcatForget(confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "取消关联"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if state.Managed {
		return "", errors.New("受管 NapCat 不能取消关联；请使用卸载")
	}
	return "✓ 已取消外部 NapCat 关联，未删除任何文件。", saveState(State{})
}

func installAction(params map[string]string, confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "安装 NapCat"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if state.InstallDir != "" && !state.Managed {
		return "", errors.New("已关联外部 NapCat；请先取消关联，工作台才可创建受管安装")
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return "", errors.New("当前系统请使用工作台下载并打开官方 NapCat 启动器；工作台不会修改 QQ 注入文件")
	}
	previousState := state
	installation, err := installNapCat()
	if err != nil {
		return "", err
	}
	state = State{Version: installation.Version, InstallDir: installation.InstallDir, Managed: true, Platform: napcatPlatform().Key, InstallMode: "managed", ReleaseTag: installation.ReleaseTag, Asset: installation.Asset, EnvironmentMode: installation.EnvironmentMode, FallbackReason: installation.FallbackReason, EnvironmentDiagnostic: installation.EnvironmentDiagnostic}
	if err := saveState(state); err != nil {
		_ = rollbackNapcatInstallation(installation)
		_ = saveState(previousState)
		return "", err
	}
	if runtime.GOOS == "linux" {
		reportNapcatProgress("start", 85, "自动启动 NapCat 并等待二维码")
		process, startErr := startNapCat(state)
		if startErr != nil {
			_ = rollbackNapcatInstallation(installation)
			_ = saveState(previousState)
			return "", fmt.Errorf("NapCat 自动启动失败，已恢复安装前状态：%w", startErr)
		}
		state.PID, state.ProcessGroupID = process.PID, process.ProcessGroupID
		if err := saveState(state); err != nil {
			stopProcess(process.ProcessGroupID)
			_ = rollbackNapcatInstallation(installation)
			_ = saveState(previousState)
			return "", err
		}
		if !waitNapcatWebUI(webUIStartupTimeout) {
			stopProcess(process.ProcessGroupID)
			_ = rollbackNapcatInstallation(installation)
			_ = saveState(previousState)
			return "", errors.New("NapCat 未能启动管理页面，安装已自动恢复到安装前状态，请查看执行日志")
		}
		discardNapcatBackup(installation)
		reportNapcatProgress("complete", 100, "NapCat 已安装并启动，等待扫码登录")
		return fmt.Sprintf("✓ NapCat 已安装并启动（版本 %s）。\n✓ 正在等待扫码登录，二维码会自动显示。", installation.Version), nil
	}
	discardNapcatBackup(installation)
	reportNapcatProgress("complete", 100, "NapCat 安装完成")
	return fmt.Sprintf("✓ NapCat 已安装（版本 %s）。\n✓ 现在可以点击「启动」运行它。", installation.Version), nil
}

func uninstallAction(confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "卸载 NapCat"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if err := requireManagedNapcat(state, "卸载"); err != nil {
		return "", err
	}
	if managedNapcatGroupAlive(state) {
		stopProcess(napcatProcessGroup(state))
		if managedNapcatGroupAlive(state) {
			return "", errors.New("NapCat 进程组未能停止，已拒绝删除安装目录")
		}
	}
	want, err := managedInstallDir()
	if err != nil {
		return "", err
	}
	if filepath.Clean(want) != filepath.Clean(state.InstallDir) {
		return "", errors.New("NapCat 受管目录不匹配，已拒绝删除")
	}
	if err := os.RemoveAll(want); err != nil {
		return "", err
	}
	return "✓ NapCat 已卸载。", saveState(State{})
}

func napcatProcessGroup(state State) int {
	if state.ProcessGroupID > 0 {
		return state.ProcessGroupID
	}
	return state.PID
}

func startAction(confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "启动 NapCat"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if err := requireManagedNapcat(state, "启动"); err != nil {
		return "", err
	}
	if isRunning(state) {
		return "? NapCat 已经在运行中。", nil
	}
	if managedNapcatGroupAlive(state) {
		stopProcess(napcatProcessGroup(state))
		if managedNapcatGroupAlive(state) {
			return "", errors.New("上一次 NapCat 进程组未能停止，已取消启动")
		}
	}
	reportNapcatProgress("start", 85, "启动 NapCat 受管进程组")
	process, err := startNapCat(state)
	if err != nil {
		return "", err
	}
	state.PID, state.ProcessGroupID = process.PID, process.ProcessGroupID
	if err := saveState(state); err != nil {
		stopProcess(process.ProcessGroupID)
		return "", err
	}
	if !waitNapcatWebUI(webUIStartupTimeout) {
		stopProcess(process.ProcessGroupID)
		state.PID, state.ProcessGroupID = 0, 0
		_ = saveState(state)
		return "", errors.New("NapCat 未能启动管理页面，已停止受管进程组，请查看执行日志")
	}
	return fmt.Sprintf("✓ NapCat 已启动（PID %d）。\n✓ 现在请用手机 QQ 扫码登录。", process.PID), nil
}

func stopAction(confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "停止 NapCat"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if err := requireManagedNapcat(state, "停止"); err != nil {
		return "", err
	}
	if !managedNapcatGroupAlive(state) {
		return "? NapCat 当前没有在运行。", nil
	}
	stopProcess(napcatProcessGroup(state))
	if isRunning(state) {
		return "", errors.New("NapCat 进程组未能停止；状态已保留以便继续诊断")
	}
	state.PID, state.ProcessGroupID = 0, 0
	if err := saveState(state); err != nil {
		return "", err
	}
	return "✓ NapCat 已停止。", nil
}

func restartAction(confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "重启 NapCat"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if err := requireManagedNapcat(state, "重启"); err != nil {
		return "", err
	}
	if managedNapcatGroupAlive(state) {
		stopProcess(napcatProcessGroup(state))
		if managedNapcatGroupAlive(state) {
			return "", errors.New("NapCat 进程组未能停止，已取消重启")
		}
	}
	process, err := startNapCat(state)
	if err != nil {
		return "", err
	}
	state.PID, state.ProcessGroupID = process.PID, process.ProcessGroupID
	if err := saveState(state); err != nil {
		stopProcess(process.ProcessGroupID)
		return "", err
	}
	timeWait(1500)
	return fmt.Sprintf("✓ NapCat 已重启（PID %d）。\n%s", process.PID, statusLine(state)), nil
}

func updateNapcat(confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "更新 NapCat"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if err := requireManagedNapcat(state, "更新"); err != nil {
		return "", err
	}
	wasRunning := isRunning(state)
	if managedNapcatGroupAlive(state) {
		stopProcess(napcatProcessGroup(state))
		if managedNapcatGroupAlive(state) {
			return "", errors.New("旧 NapCat 进程组未能停止，未开始更新")
		}
		// Persist the stopped state before a lengthy download. This makes a
		// watchdog observation unambiguously intentional even if it starts after
		// the lifecycle lock is released unexpectedly.
		state.PID, state.ProcessGroupID = 0, 0
		if err := saveState(state); err != nil {
			if wasRunning {
				if process, startErr := startNapCat(state); startErr == nil {
					state.PID, state.ProcessGroupID = process.PID, process.ProcessGroupID
					_ = saveState(state)
				}
			}
			return "", fmt.Errorf("无法记录 NapCat 已停止状态，未开始更新：%w", err)
		}
	}
	installation, installErr := installNapCat()
	if installErr != nil {
		if wasRunning {
			if process, startErr := startNapCat(state); startErr == nil {
				state.PID, state.ProcessGroupID = process.PID, process.ProcessGroupID
				_ = saveState(state)
			}
		}
		return "", installErr
	}
	updated := State{Version: installation.Version, InstallDir: installation.InstallDir, Managed: true, Platform: napcatPlatform().Key, InstallMode: "managed", ReleaseTag: installation.ReleaseTag, Asset: installation.Asset, EnvironmentMode: installation.EnvironmentMode, FallbackReason: installation.FallbackReason, EnvironmentDiagnostic: installation.EnvironmentDiagnostic, WatchdogPID: state.WatchdogPID}
	if wasRunning {
		reportNapcatProgress("restart", 90, "恢复更新后的 NapCat 运行状态")
		process, startErr := startNapCat(updated)
		if startErr != nil {
			if rollbackErr := rollbackNapcatInstallation(installation); rollbackErr != nil {
				return "", fmt.Errorf("更新后的 NapCat 无法启动（%v），且回滚失败：%w", startErr, rollbackErr)
			}
			if process, oldStartErr := startNapCat(state); oldStartErr == nil {
				state.PID, state.ProcessGroupID = process.PID, process.ProcessGroupID
				_ = saveState(state)
			}
			return "", fmt.Errorf("更新后的 NapCat 无法启动，已恢复旧版本：%w", startErr)
		}
		updated.PID, updated.ProcessGroupID = process.PID, process.ProcessGroupID
		if !waitNapcatWebUI(webUIStartupTimeout) {
			stopProcess(process.ProcessGroupID)
			if rollbackErr := rollbackNapcatInstallation(installation); rollbackErr != nil {
				return "", fmt.Errorf("更新后的 NapCat 未能启动管理页面，且回滚失败：%w", rollbackErr)
			}
			if process, oldStartErr := startNapCat(state); oldStartErr == nil {
				state.PID, state.ProcessGroupID = process.PID, process.ProcessGroupID
				_ = saveState(state)
			}
			return "", errors.New("更新后的 NapCat 未能启动管理页面，已恢复旧版本")
		}
	}
	if err := saveState(updated); err != nil {
		if wasRunning {
			stopProcess(napcatProcessGroup(updated))
		}
		if rollbackErr := rollbackNapcatInstallation(installation); rollbackErr == nil && wasRunning {
			if process, oldStartErr := startNapCat(state); oldStartErr == nil {
				state.PID, state.ProcessGroupID = process.PID, process.ProcessGroupID
				_ = saveState(state)
			}
		}
		return "", err
	}
	discardNapcatBackup(installation)
	reportNapcatProgress("complete", 100, "NapCat 更新完成")
	return fmt.Sprintf("✓ NapCat 已更新到 %s。", updated.Version), nil
}

func logAction(params map[string]string) (string, error) {
	lines, err := linesParam(params)
	if err != nil {
		return "", err
	}
	return tailLog(lines)
}

func timeWait(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

func waitNapcatWebUI(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if webUIBridge() != "" {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
