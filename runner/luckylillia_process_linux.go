//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// startLuckyProcess intentionally does not call the upstream start.sh. A
// workbench-managed process has no TTY, so the script's interactive branch is
// not a reliable contract. The CLI itself starts its WebUI in direct mode;
// PMHQ is a separate upstream service and must never be implied by an orphan
// --pmhq flag on this single process.
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
	command := exec.Command(binary)
	command.Dir = root
	command.Stdout, command.Stderr, command.Stdin = log, log, nil
	detachProcess(command)
	if err := command.Start(); err != nil {
		return luckyProcess{}, fmt.Errorf("启动 LuckyLillia 登录服务失败：%w", err)
	}
	pid := command.Process.Pid
	_ = command.Process.Release()
	return luckyProcess{PID: pid, ProcessGroupID: pid}, nil
}
