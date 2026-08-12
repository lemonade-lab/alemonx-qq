//go:build !windows

package main

import "fmt"

func windowsInstallerReady() bool       { return false }
func windowsInstallerPath() string      { return "" }
func windowsNapcatLauncherPath() string { return "" }
func downloadWindowsNapcatInstaller() (string, error) {
	return "", fmt.Errorf("当前系统不支持 Windows NapCat 安装器")
}
func openWindowsNapcatLauncher() (string, error) {
	return "", fmt.Errorf("当前系统不支持 Windows NapCat 启动器")
}
