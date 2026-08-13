//go:build linux

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// startNapCat manages the X display server and QQ directly, without a shell
// wrapper or any package-provided script.
func startNapCat(state State) (napcatProcess, error) {
	// A bare TCP probe is the available cross-platform readiness signal. Refuse
	// to start when 6099 is already occupied so a different local service cannot
	// make installation or an update look successful.
	if webUIBridge() != "" {
		return napcatProcess{}, fmt.Errorf("NapCat 管理面板端口 %d 已被其他进程占用", webUIPort)
	}
	qq, err := linuxQQBinary(state)
	if err != nil {
		return napcatProcess{}, err
	}
	environment, err := linuxEnvironmentForState(state)
	if err != nil {
		return napcatProcess{}, err
	}
	logFile, err := logPath()
	if err != nil {
		return napcatProcess{}, err
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		return napcatProcess{}, err
	}
	logHandle, err := openAppendLog(logFile)
	if err != nil {
		return napcatProcess{}, err
	}
	defer logHandle.Close()
	display, err := availableXDisplay()
	if err != nil {
		return napcatProcess{}, err
	}
	var xvfb *exec.Cmd
	if environment.Runtime != nil {
		xvfb = managedRuntimeCommand(*environment.Runtime, environment.Runtime.Xvfb, display, "-screen", "0", "1280x720x24", "-nolisten", "tcp", "-ac")
	} else {
		xvfb = exec.Command("Xvfb", display, "-screen", "0", "1280x720x24", "-nolisten", "tcp", "-ac")
	}
	xvfb.Stdout, xvfb.Stderr, xvfb.Stdin = logHandle, logHandle, nil
	detachProcess(xvfb)
	if err := xvfb.Start(); err != nil {
		return napcatProcess{}, fmt.Errorf("启动 Xvfb 失败：%w", err)
	}
	groupID := xvfb.Process.Pid
	stopXvfb := func() { stopManagedProcess(groupID) }
	if !waitXDisplay(display, 5*time.Second) {
		stopXvfb()
		return napcatProcess{}, errors.New("Xvfb 未能在 5 秒内就绪，请查看 NapCat 日志")
	}
	// Chromium refuses to run its sandbox as UID 0. Retain the sandbox for
	// ordinary service accounts; root installations need this explicit fallback
	// and the fact is visible in the operation log.
	args := []string{}
	if os.Geteuid() == 0 {
		args = append(args, "--no-sandbox")
		_, _ = fmt.Fprintln(logHandle, "NapCat: 检测到 root，QQ 将以 --no-sandbox 启动；建议生产环境使用普通服务用户。")
	}
	var qqCommand *exec.Cmd
	if environment.Runtime != nil {
		qqCommand = managedRuntimeCommand(*environment.Runtime, qq, args...)
	} else {
		qqCommand = exec.Command(qq, args...)
	}
	qqCommand.Dir = filepath.Dir(qq)
	// NAPCAT_HOME also repairs installations created by older runners that
	// embedded their now-removed staging directory in loadNapCat.js.
	qqCommand.Env = append(os.Environ(), "DISPLAY="+display, "NAPCAT_HOME="+state.InstallDir)
	qqCommand.Stdout, qqCommand.Stderr, qqCommand.Stdin = logHandle, logHandle, nil
	joinProcessGroup(qqCommand, groupID)
	if err := qqCommand.Start(); err != nil {
		stopXvfb()
		return napcatProcess{}, fmt.Errorf("启动 QQ/NapCat 失败：%w", err)
	}
	pid := qqCommand.Process.Pid
	_ = xvfb.Process.Release()
	_ = qqCommand.Process.Release()
	return napcatProcess{PID: pid, ProcessGroupID: groupID}, nil
}

func linuxQQBinary(state State) (string, error) {
	if state.InstallDir == "" {
		return "", errors.New("未记录 NapCat Linux 安装目录")
	}
	if err := validateLinuxQQRuntime(state.InstallDir); err != nil {
		return "", err
	}
	qq := filepath.Join(state.InstallDir, "opt", "QQ", "qq")
	info, err := os.Stat(qq)
	if err != nil || info.IsDir() {
		return "", errors.New("未找到受管 QQ 启动文件：" + qq)
	}
	return qq, nil
}

func availableXDisplay() (string, error) {
	for number := 90; number < 190; number++ {
		path := filepath.Join("/tmp/.X11-unix", "X"+strconv.Itoa(number))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return ":" + strconv.Itoa(number), nil
		}
	}
	return "", errors.New("没有可用的本地 Xvfb 显示编号")
}

func waitXDisplay(display string, timeout time.Duration) bool {
	path := filepath.Join("/tmp/.X11-unix", "X"+display[1:])
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("unix", path, 200*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func stopProcess(pid int) { stopManagedProcess(pid) }

// isRunning deliberately checks QQ rather than the Xvfb helper: a crashed QQ
// must be reported as stopped even when its display server still exists.
func isRunning(state State) bool { return processAlive(state.PID) }

func platformInstallDir() (string, error) { return managedInstallDir() }

func macInstallGuide() (string, error) {
	return "", fmt.Errorf("当前系统不支持 macOS 安装方式")
}
func downloadMacNapcatInstaller() (string, error) {
	return "", fmt.Errorf("当前系统不支持 macOS NapCat 安装器")
}
func macNapcatVersion() string { return "" }
func openMacNapcatInstaller() (string, error) {
	return "", fmt.Errorf("当前系统不支持 macOS NapCat 安装器")
}
func macInstallerReady() bool       { return false }
func macInstallerPath() string      { return "" }
func macNapcatLauncherPath() string { return "" }
func openMacNapcatLauncher() (string, error) {
	return "", fmt.Errorf("当前系统不支持 macOS NapCat 启动器")
}
func macQQInstalled() bool    { return false }
func macNapcatInjected() bool { return false }
func macInstallDir() (string, error) {
	return "", fmt.Errorf("当前系统不支持 macOS 安装方式")
}
