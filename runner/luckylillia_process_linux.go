//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// startLuckyProcess intentionally does not call the upstream start.sh. That
// script selects token-only headless mode when its stdin/stdout have no TTY,
// which is always true for a workbench-managed process. We reproduce its
// documented headed Shell path directly and own the Xvfb process ourselves.
func startLuckyProcess(platform *luckyPlatformSpec, root, entry string, log *os.File) (luckyProcess, error) {
	if platform == nil {
		return luckyProcess{}, errors.New("当前平台没有 LuckyLillia CLI 启动契约")
	}
	if platform.Key != "linux-amd64" && platform.Key != "linux-arm64" {
		return startLuckyProcessDefault(platform, root, entry, log)
	}
	binary := filepath.Join(root, platform.CLIBinary)
	info, err := os.Stat(binary)
	if err != nil || info.IsDir() {
		return luckyProcess{}, errors.New("LuckyLillia CLI 包缺少 llbot 启动程序")
	}
	// Archives do not always preserve mode bits when they are unpacked through
	// different clients. This is the same local preparation the official script
	// performs before launching LLBot.
	if err := os.Chmod(binary, info.Mode()|0o700); err != nil {
		return luckyProcess{}, fmt.Errorf("准备 LuckyLillia 启动程序失败：%w", err)
	}
	environment, err := prepareLinuxEnvironment(false)
	if err != nil {
		return luckyProcess{}, err
	}
	display, err := availableXDisplay()
	if err != nil {
		return luckyProcess{}, err
	}
	var xvfb *exec.Cmd
	if environment.Runtime != nil {
		xvfb = managedRuntimeCommand(*environment.Runtime, environment.Runtime.Xvfb, display, "-screen", "0", "1280x720x24", "-nolisten", "tcp", "-ac")
	} else {
		xvfb = exec.Command("Xvfb", display, "-screen", "0", "1280x720x24", "-nolisten", "tcp", "-ac")
	}
	xvfb.Stdout, xvfb.Stderr, xvfb.Stdin = log, log, nil
	detachProcess(xvfb)
	if err := xvfb.Start(); err != nil {
		return luckyProcess{}, fmt.Errorf("启动 LuckyLillia 图形环境失败：%w", err)
	}
	groupID := xvfb.Process.Pid
	stopGroup := func() { stopManagedProcess(groupID) }
	if !waitXDisplay(display, 5*time.Second) {
		stopGroup()
		return luckyProcess{}, errors.New("LuckyLillia 图形环境未能就绪，请查看日志")
	}
	var command *exec.Cmd
	if environment.Runtime != nil {
		command = managedRuntimeCommand(*environment.Runtime, binary, "--pmhq")
	} else {
		command = exec.Command(binary, "--pmhq")
	}
	command.Dir = root
	command.Env = append(os.Environ(), "DISPLAY="+display)
	command.Stdout, command.Stderr, command.Stdin = log, log, nil
	joinProcessGroup(command, groupID)
	if err := command.Start(); err != nil {
		stopGroup()
		return luckyProcess{}, fmt.Errorf("启动 LuckyLillia 登录服务失败：%w", err)
	}
	pid := command.Process.Pid
	_ = xvfb.Process.Release()
	_ = command.Process.Release()
	return luckyProcess{PID: pid, ProcessGroupID: groupID}, nil
}
