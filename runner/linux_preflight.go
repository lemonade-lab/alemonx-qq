//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// linuxHostCompatibility describes the native path that this runner has
// actually packaged and tested. It is a preflight, not a promise that every
// Linux distribution with an X server behaves identically.
type linuxHostCompatibility struct {
	Distribution    string
	Version         string
	PackageManager  string
	Libc            string
	Container       bool
	NativeSupported bool
	Diagnostic      string
}

func currentLinuxHostCompatibility() linuxHostCompatibility {
	profile := linuxHostCompatibility{Distribution: "Linux", Libc: "glibc"}
	values := readOSRelease("/etc/os-release")
	if id := values["ID"]; id != "" {
		profile.Distribution = id
	}
	profile.Version = values["VERSION_ID"]
	if _, err := exec.LookPath("apt-get"); err == nil {
		profile.PackageManager = "apt"
	} else if _, err := exec.LookPath("dnf"); err == nil {
		profile.PackageManager = "dnf"
	}
	if matches, _ := filepath.Glob("/lib/ld-musl-*.so.1"); len(matches) > 0 {
		profile.Libc = "musl"
	}
	profile.Container = fileExists("/.dockerenv") || fileExists("/run/.containerenv") || processLooksContainerized()
	switch {
	case runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64":
		profile.Diagnostic = "当前 Linux 架构不在 NapCat / LLBot 官方自动安装范围内（仅 amd64、arm64）。"
	case profile.Libc != "glibc":
		profile.Diagnostic = "检测到 musl/Alpine；官方 Linux QQ 是 glibc Electron 运行时，当前原生安装已阻止。请改用受支持的 glibc 发行版或独立 Docker 部署。"
	case profile.PackageManager == "":
		profile.Diagnostic = "未检测到 APT 或 DNF；当前原生安装无法选择经验证的 QQ DEB/RPM 包。"
	default:
		profile.NativeSupported = true
		profile.Diagnostic = "Linux 原生安装路径可用（" + profile.PackageManager + "+glibc）。"
	}
	if profile.Container && profile.NativeSupported {
		profile.Diagnostic += " 当前运行在容器中；请确保容器具备 /dev/shm、Xvfb 和所需系统库。"
	}
	return profile
}

func linuxPreflightError() error {
	profile := currentLinuxHostCompatibility()
	if profile.NativeSupported {
		return nil
	}
	return &linuxCompatibilityError{message: profile.Diagnostic}
}

type linuxCompatibilityError struct{ message string }

func (e *linuxCompatibilityError) Error() string { return e.message }

func readOSRelease(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		values[key] = strings.Trim(strings.TrimSpace(value), "\"")
	}
	return values
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func processLooksContainerized() bool {
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	value := strings.ToLower(string(data))
	return strings.Contains(value, "docker") || strings.Contains(value, "containerd") || strings.Contains(value, "kubepods") || strings.Contains(value, "libpod")
}
