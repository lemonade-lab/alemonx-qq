//go:build !linux

package main

import "fmt"

// Keep the action installer cross-compilable. These values can never be
// returned on non-Linux platforms because installNapCat dispatches before
// environment preparation.
type managedLinuxRuntime struct {
	ID          string
	Asset       string
	SHA256      string
	Fingerprint string
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

func loadManagedLinuxRuntime(string, string, string) (managedLinuxRuntime, error) {
	return managedLinuxRuntime{}, fmt.Errorf("当前系统不支持 Linux 兼容运行环境")
}

func linuxRuntimeStateFingerprint(installationFingerprint, _ string) string {
	return installationFingerprint
}
