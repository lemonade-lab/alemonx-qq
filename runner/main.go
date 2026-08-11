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
	output, err := run(input.Action, input.Params, input.Confirm)
	write(response{Output: output, Error: errorText(err)})
}

func write(result response) { _ = json.NewEncoder(os.Stdout).Encode(result) }
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func run(action string, params map[string]string, confirmed bool) (string, error) {
	switch action {
	case "status", "napcat-status":
		return statusAction()
	case "napcat-qrcode":
		return napcatQRCodeAction()
	case "napcat-adopt":
		return napcatAdopt(params, confirmed)
	case "napcat-forget":
		return napcatForget(confirmed)
	case "install":
		return installAction(confirmed)
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
	case "luckylillia-onebot-config":
		return luckyOneBotConfig()
	case "luckylillia-onebot-set":
		return luckySetOneBotConfig(params, confirmed)
	default:
		return "", fmt.Errorf("未知操作：%s", action)
	}
}

func statusAction() (string, error) { return statusJSON() }

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
	fingerprint, err := napcatFingerprint(dir)
	if err != nil {
		return "", err
	}
	platform := napcatPlatform()
	label := "当前平台"
	platformKey := "external"
	if platform != nil {
		label = platform.Label
		platformKey = platform.Key
	}
	state = State{InstallDir: dir, InstallMode: "external", Managed: false, Platform: platformKey, Fingerprint: fingerprint}
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

func installAction(confirmed bool) (string, error) {
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
	if runtime.GOOS == "darwin" {
		return "", errors.New("macOS NapCat 仅支持外部关联；工作台不会修改 QQ 注入文件")
	}
	installation, err := installNapCat()
	if err != nil {
		return "", err
	}
	state = State{Version: installation.Version, InstallDir: installation.InstallDir, Managed: true, Platform: napcatPlatform().Key, InstallMode: "managed", ReleaseTag: installation.ReleaseTag, Asset: installation.Asset, ArchiveSHA256: installation.ArchiveSHA256, Fingerprint: installation.Fingerprint}
	if err := saveState(state); err != nil {
		_ = rollbackNapcatInstallation(installation)
		return "", err
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
	if isRunning(state) {
		stopProcess(napcatProcessGroup(state))
		if isRunning(state) {
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
	reportNapcatProgress("start", 85, "启动 NapCat 受管进程组")
	pid, err := startNapCat(state)
	if err != nil {
		return "", err
	}
	state.PID, state.ProcessGroupID = pid, pid
	if err := saveState(state); err != nil {
		stopProcess(pid)
		return "", err
	}
	timeWait(1500)
	if url := webUIBridge(); url != "" {
		return fmt.Sprintf("✓ NapCat 已启动（PID %d）。\n✓ 管理面板可访问：%s\n✓ 用手机 QQ 扫码登录后即可使用。", pid, url), nil
	}
	return fmt.Sprintf("✓ NapCat 已启动（PID %d）。\n? 管理面板（6099）尚未就绪，请稍等片刻后查看状态。", pid), nil
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
	if !isRunning(state) {
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
	if isRunning(state) {
		stopProcess(napcatProcessGroup(state))
		if isRunning(state) {
			return "", errors.New("NapCat 进程组未能停止，已取消重启")
		}
	}
	pid, err := startNapCat(state)
	if err != nil {
		return "", err
	}
	state.PID, state.ProcessGroupID = pid, pid
	if err := saveState(state); err != nil {
		stopProcess(pid)
		return "", err
	}
	timeWait(1500)
	return fmt.Sprintf("✓ NapCat 已重启（PID %d）。\n%s", pid, statusLine(state)), nil
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
	if wasRunning {
		stopProcess(napcatProcessGroup(state))
		if isRunning(state) {
			return "", errors.New("旧 NapCat 进程组未能停止，未开始更新")
		}
	}
	installation, installErr := installNapCat()
	if installErr != nil {
		if wasRunning {
			if pid, startErr := startNapCat(state); startErr == nil {
				state.PID, state.ProcessGroupID = pid, pid
				_ = saveState(state)
			}
		}
		return "", installErr
	}
	updated := State{Version: installation.Version, InstallDir: installation.InstallDir, Managed: true, Platform: napcatPlatform().Key, InstallMode: "managed", ReleaseTag: installation.ReleaseTag, Asset: installation.Asset, ArchiveSHA256: installation.ArchiveSHA256, Fingerprint: installation.Fingerprint, WatchdogPID: state.WatchdogPID}
	if wasRunning {
		reportNapcatProgress("restart", 90, "恢复更新后的 NapCat 运行状态")
		pid, startErr := startNapCat(updated)
		if startErr != nil {
			if rollbackErr := rollbackNapcatInstallation(installation); rollbackErr != nil {
				return "", fmt.Errorf("更新后的 NapCat 无法启动（%v），且回滚失败：%w", startErr, rollbackErr)
			}
			if pid, oldStartErr := startNapCat(state); oldStartErr == nil {
				state.PID, state.ProcessGroupID = pid, pid
				_ = saveState(state)
			}
			return "", fmt.Errorf("更新后的 NapCat 无法启动，已恢复旧版本：%w", startErr)
		}
		updated.PID, updated.ProcessGroupID = pid, pid
	}
	if err := saveState(updated); err != nil {
		if wasRunning {
			stopProcess(napcatProcessGroup(updated))
		}
		if rollbackErr := rollbackNapcatInstallation(installation); rollbackErr == nil && wasRunning {
			if pid, oldStartErr := startNapCat(state); oldStartErr == nil {
				state.PID, state.ProcessGroupID = pid, pid
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
