//go:build !linux

package main

import "fmt"

// Keep the action installer cross-compilable. These values can never be
// returned on non-Linux platforms because installNapCat dispatches before
// environment preparation.
type managedLinuxRuntime struct {
}

type linuxEnvironment struct {
	Mode       string
	Runtime    *managedLinuxRuntime
	Reason     string
	Diagnostic string
}

func prepareLinuxEnvironment(bool) (linuxEnvironment, error) {
	return linuxEnvironment{}, fmt.Errorf("当前系统不支持 Linux 兼容运行环境")
}

func loadManagedLinuxRuntime() (managedLinuxRuntime, error) {
	return managedLinuxRuntime{}, fmt.Errorf("当前系统不支持 Linux 兼容运行环境")
}
