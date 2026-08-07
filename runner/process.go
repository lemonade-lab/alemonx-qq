package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const webUIPort = 6099

// webUIBridge returns the URL a user scans/logs into, or "" if not reachable.
func webUIBridge() string {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(webUIPort)), 300*time.Millisecond)
	if err != nil {
		return ""
	}
	_ = connection.Close()
	return "http://127.0.0.1:" + strconv.Itoa(webUIPort) + "/webui"
}

// napcatCommand builds the command that launches NapCat in the background.
// The runner detaches itself as the parent so the NapCat process survives alx.
func napcatCommand(state State) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "windows":
		return windowsNapcatCommand(state)
	case "linux":
		return linuxNapcatCommand(state)
	default:
		return nil, fmt.Errorf("当前系统暂不支持 NapCat（仅 Windows / Linux）")
	}
}

// startNapCat spawns NapCat detached from the runner's process group.
func startNapCat(state State) (int, error) {
	command, err := napcatCommand(state)
	if err != nil {
		return 0, err
	}
	logFile, err := logPath()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return 0, err
	}
	handle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, err
	}
	defer handle.Close()
	command.Stdin = nil
	command.Stdout = handle
	command.Stderr = handle
	detachProcess(command)
	if err := command.Start(); err != nil {
		return 0, err
	}
	pid := command.Process.Pid
	_ = command.Process.Release()
	return pid, nil
}

func processAlive(pid int) bool {
	return aliveProbe(pid)
}

func stopProcess(pid int) {
	if pid <= 0 {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = process.Signal(os.Interrupt)
	time.Sleep(500 * time.Millisecond)
	if aliveProbe(pid) {
		_ = process.Kill()
	}
}

// status returns a human-readable status line for the current state.
func statusLine(state State) string {
	lines := []string{}
	if state.InstallDir == "" || !dirExists(state.InstallDir) {
		return "? 未安装 NapCat\n? WebUI 未运行"
	}
	lines = append(lines, "✓ 已安装 NapCat")
	if state.Version != "" {
		lines = append(lines, "  版本："+state.Version)
	}
	if processAlive(state.PID) {
		lines = append(lines, fmt.Sprintf("✓ 进程运行中（PID %d）", state.PID))
	} else {
		lines = append(lines, "? 进程未运行")
	}
	if url := webUIBridge(); url != "" {
		lines = append(lines, "✓ 管理面板可访问："+url)
	} else {
		lines = append(lines, "? 管理面板（6099）未连接")
	}
	return strings.Join(lines, "\n")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func windowsNapcatCommand(state State) (*exec.Cmd, error) {
	launcher := filepath.Join(state.InstallDir, "launcher.bat")
	if _, err := os.Stat(launcher); err != nil {
		exe := filepath.Join(state.InstallDir, "launcher.exe")
		if _, statErr := os.Stat(exe); statErr != nil {
			return nil, errors.New("未找到 NapCat 启动器（launcher.bat / launcher.exe）")
		}
		return exec.Command(exe), nil
	}
	return exec.Command("cmd", "/c", launcher), nil
}

func linuxNapcatCommand(state State) (*exec.Cmd, error) {
	if _, err := exec.LookPath("napcat"); err != nil {
		return nil, errors.New("未找到 napcat 命令；请先安装 NapCat-Installer")
	}
	return exec.Command("napcat", "start"), nil
}
