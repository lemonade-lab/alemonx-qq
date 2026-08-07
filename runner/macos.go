//go:build darwin

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// macOS runs NapCat by injecting it into the installed QQ app (Electron), so
// there is no detached NapCat process to manage. "start" opens QQ (NapCat runs
// inside it); "stop" quits QQ via AppleScript; running is signalled by the
// 6099 WebUI being reachable.

const (
	macQQApp       = "/Applications/QQ.app"
	macQQContainer = "Library/Containers/com.tencent.qq/Data"
)

// macNapcatContainer returns the NapCat sandbox container path for QQ.
func macNapcatContainer() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, macQQContainer)
}

func macQQInstalled() bool {
	info, err := os.Stat(macQQApp)
	return err == nil && info.IsDir()
}

// macNapcatInjected reports whether NapCat has been injected into QQ's
// sandbox container (the installer writes loadNapCat.js / napcat files there).
func macNapcatInjected() bool {
	container := macNapcatContainer()
	if container == "" {
		return false
	}
	for _, marker := range []string{"loadNapCat.js", "napcat"} {
		if _, err := os.Stat(filepath.Join(container, marker)); err == nil {
			return true
		}
	}
	return false
}

// macInstallDir is the NapCat directory inside the QQ container.
func macInstallDir() (string, error) {
	container := macNapcatContainer()
	if container == "" {
		return "", fmt.Errorf("无法定位 QQ 沙箱容器")
	}
	dir := filepath.Join(container, "napcat")
	if !dirExists(dir) {
		return dir, nil
	}
	return dir, nil
}

// macNapcatVersion reads the NapCat version from the injected package.json.
func macNapcatVersion() string {
	dir, err := macInstallDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return ""
	}
	return manifest.Version
}

// macInstallGuide returns guidance for installing NapCat on macOS.
func macInstallGuide() (string, error) {
	if !macQQInstalled() {
		return "", fmt.Errorf("未检测到 QQ；请先从 Mac App Store 安装 QQ（≥ 9.9.27），然后重试")
	}
	if macNapcatInjected() {
		version := macNapcatVersion()
		if version != "" {
			return version, nil
		}
		return "injected", nil
	}
	hint := strings.Join([]string{
		"在 macOS 上，NapCat 通过注入 QQ 应用运行（无需 Docker）。",
		"安装步骤：",
		"1. 安装 QQ（Mac App Store，≥ 9.9.27）与 Node.js 18+。",
		"2. 下载 NapCat-Mac-Installer（https://github.com/NapNeko/NapCat-Mac-Installer/releases）。",
		"3. 打开 NapCat安装器.app，点「安装」；按提示在「系统设置 → 隐私与安全性 → App 管理」中授权。",
		"4. 安装完成后回到本插件，点「启动」运行 QQ（NapCat 会随之启动）。",
	}, "\n")
	return "", fmt.Errorf("%s\n（完成上述步骤后重试本操作）", hint)
}

// macStartNapCat opens QQ; NapCat (injected) starts along with it.
func macStartNapCat() error {
	output, err := exec.Command("open", "-a", "QQ").CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			text = err.Error()
		}
		return fmt.Errorf("启动 QQ 失败：%s", text)
	}
	// Wait briefly for NapCat's WebUI (6099) to come up.
	for i := 0; i < 20; i++ {
		if webUIBridge() != "" {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// macStopNapCat quits QQ via AppleScript.
func macStopNapCat() error {
	output, err := exec.Command("osascript", "-e", `tell application "QQ" to quit`).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			text = err.Error()
		}
		return fmt.Errorf("退出 QQ 失败：%s", text)
	}
	return nil
}

// startNapCat opens QQ on macOS (NapCat runs inside it). No PID is tracked
// because QQ is a foreground app, not a detached process.
func startNapCat(state State) (int, error) {
	if err := macStartNapCat(); err != nil {
		return 0, err
	}
	return 0, nil
}

// stopProcess quits QQ on macOS.
func stopProcess(pid int) {
	_ = macStopNapCat()
}

// isRunning on macOS means the injected NapCat's WebUI is reachable.
func isRunning(state State) bool {
	return webUIBridge() != ""
}

// platformInstallDir on macOS is the NapCat directory inside the QQ container.
func platformInstallDir() (string, error) {
	return macInstallDir()
}
