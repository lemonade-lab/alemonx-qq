package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// statusPayload is the structured status returned by the `status` action so
// the web UI can poll and render it precisely. Other actions keep returning
// plain ✓/!? text.
type statusPayload struct {
	Engine               string                 `json:"engine"`
	Installed            bool                   `json:"installed"`
	InstallHealthy       bool                   `json:"installHealthy"`
	Running              bool                   `json:"running"`
	PortReachable        bool                   `json:"portReachable"`
	WebUIReady           bool                   `json:"webUiReady"`
	OneBotReady          bool                   `json:"oneBotReady"`
	LoginPending         bool                   `json:"loginPending"`
	Watchdog             bool                   `json:"watchdog"`
	Version              string                 `json:"version,omitempty"`
	PID                  int                    `json:"pid,omitempty"`
	WebUIURL             string                 `json:"webUiUrl,omitempty"`
	OneBotURL            string                 `json:"oneBotUrl,omitempty"`
	QRCodeAvailable      bool                   `json:"qrCodeAvailable"`
	QRCodeUpdatedAt      string                 `json:"qrCodeUpdatedAt,omitempty"`
	InstallerReady       bool                   `json:"installerReady"`
	InstallerPath        string                 `json:"installerPath,omitempty"`
	LauncherPath         string                 `json:"launcherPath,omitempty"`
	DiagnosticHint       string                 `json:"diagnosticHint,omitempty"`
	Supported            bool                   `json:"supported"`
	Managed              bool                   `json:"managed"`
	Platform             string                 `json:"platform,omitempty"`
	InstallMode          string                 `json:"installMode,omitempty"`
	ReleaseTag           string                 `json:"releaseTag,omitempty"`
	Asset                string                 `json:"asset,omitempty"`
	ArchiveSHA256        string                 `json:"archiveSha256,omitempty"`
	RuntimeAsset         string                 `json:"runtimeAsset,omitempty"`
	RuntimeArchiveSHA256 string                 `json:"runtimeArchiveSha256,omitempty"`
	Fingerprint          string                 `json:"fingerprint,omitempty"`
	ValidatedAt          string                 `json:"validatedAt,omitempty"`
	Verified             bool                   `json:"verified"`
	VerificationReason   string                 `json:"verificationReason,omitempty"`
	ManagedActions       bool                   `json:"managedActions"`
	LinuxDependencies    *linuxDependencyStatus `json:"linuxDependencies,omitempty"`
	Accounts             []napcatAccount        `json:"accounts,omitempty"`
	SelectedAccount      string                 `json:"selectedAccount,omitempty"`
	UpdatedAt            string                 `json:"updatedAt"`
	Error                string                 `json:"error,omitempty"`
}

type napcatAccount struct {
	QQ          string `json:"qq"`
	OneBotURL   string `json:"oneBotUrl,omitempty"`
	OneBotReady bool   `json:"oneBotReady"`
}

type linuxDependencyStatus struct {
	Supported      bool     `json:"supported"`
	Ready          bool     `json:"ready"`
	PackageManager string   `json:"packageManager,omitempty"`
	SystemAccount  string   `json:"systemAccount,omitempty"`
	Missing        []string `json:"missing,omitempty"`
	Hint           string   `json:"hint,omitempty"`
}

// collectStatus gathers the live status from state + process probes.
func collectStatus(state State) statusPayload {
	platform := napcatPlatform()
	payload := statusPayload{
		Engine: "napcat", Version: state.Version, PID: state.PID,
		Managed: state.Managed, InstallMode: state.InstallMode, ReleaseTag: state.ReleaseTag,
		Asset: state.Asset, ArchiveSHA256: state.ArchiveSHA256, RuntimeAsset: state.RuntimeAsset, RuntimeArchiveSHA256: state.RuntimeArchiveSHA256, Fingerprint: state.Fingerprint,
		ValidatedAt: state.ValidatedAt, UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if platform != nil {
		payload.Supported, payload.Platform = true, platform.Key
	}
	// Verified answers the install-time question: does this platform have an
	// official automatic-install contract? ManagedActions answers the runtime
	// question: does this installed copy still match its recorded identity?
	payload.Verified = napcatVerified()
	payload.ManagedActions = state.Managed && napcatStateVerified(state)
	if !payload.Verified && platform != nil && platform.AutoInstall {
		payload.VerificationReason = napcatVerificationReason()
	}
	payload.Installed = state.InstallDir != "" && dirExists(state.InstallDir)
	if platform != nil && platform.Key == "darwin-external" && !payload.Installed && macQQInstalled() && macNapcatInjected() {
		if root, err := macInstallDir(); err == nil && dirExists(root) {
			payload.Installed, payload.InstallHealthy = true, true
		}
	}
	payload.InstallHealthy = payload.Installed
	if platform != nil && platform.Key == "darwin-external" {
		payload.InstallerReady = macInstallerReady()
		if payload.InstallerReady {
			payload.InstallerPath = macInstallerPath()
		}
		payload.LauncherPath = macNapcatLauncherPath()
	}
	if platform != nil && platform.Key == "windows-external" {
		payload.InstallerReady = windowsInstallerReady()
		if payload.InstallerReady {
			payload.InstallerPath = windowsInstallerPath()
		}
		payload.LauncherPath = windowsNapcatLauncherPath()
	}
	payload.Running = isRunning(state)
	payload.Watchdog = payload.ManagedActions && processAlive(state.WatchdogPID)
	payload.WebUIURL = webUIBridge()
	payload.PortReachable = payload.WebUIURL != ""
	payload.WebUIReady = payload.PortReachable
	if accounts, err := napcatAccounts(state); err == nil {
		payload.Accounts = accounts
		selected := state.SelectedQQ
		if selected == "" && len(accounts) == 1 {
			selected = accounts[0].QQ
		}
		for _, account := range accounts {
			if account.QQ == selected {
				payload.SelectedAccount = account.QQ
				payload.OneBotURL = account.OneBotURL
				payload.OneBotReady = account.OneBotReady
				break
			}
		}
	}
	payload.LoginPending = payload.Running && payload.WebUIReady && !payload.OneBotReady
	payload.QRCodeAvailable, payload.QRCodeUpdatedAt = napcatQRCodeStatus(state)
	payload.LinuxDependencies = napcatLinuxDependencies()

	reasons := []string{}
	if !payload.Supported {
		reasons = append(reasons, "当前平台不支持 NapCat")
	} else if !payload.Installed {
		reasons = append(reasons, "未安装")
	}
	if payload.Installed && !state.Managed {
		reasons = append(reasons, "这是外部关联实例，自动操作已禁用")
	}
	if state.Managed && !payload.ManagedActions {
		reasons = append(reasons, "当前受管安装信息或运行文件已变化")
	}
	if payload.Installed && !payload.Running {
		reasons = append(reasons, "进程未运行")
	}
	if payload.Installed && payload.Running && !payload.PortReachable {
		reasons = append(reasons, "管理面板（6099）不可达")
	}
	if payload.LoginPending {
		payload.DiagnosticHint = "NapCat 已启动，等待在 WebUI 中扫码登录；OneBot 服务会在登录后就绪。"
	} else if payload.VerificationReason != "" {
		payload.DiagnosticHint = payload.VerificationReason
	}
	payload.Error = strings.Join(reasons, "；")
	return payload
}

func napcatLinuxDependencies() *linuxDependencyStatus {
	installed := dpkgPackageInstalled
	if _, err := exec.LookPath("apt-get"); err != nil {
		installed = rpmPackageInstalled
	}
	return napcatLinuxDependenciesFor(runtime.GOOS, exec.LookPath, installed)
}

func napcatLinuxDependenciesFor(goos string, lookPath func(string) (string, error), installed func(string) bool) *linuxDependencyStatus {
	if goos != "linux" {
		return nil
	}
	packageManager := ""
	if _, err := lookPath("apt-get"); err == nil {
		packageManager = "apt"
	} else if _, err := lookPath("dnf"); err == nil {
		packageManager = "dnf"
	} else {
		return &linuxDependencyStatus{Hint: "未检测到 APT 或 DNF；当前 Linux 发行版暂不能自动补齐 NapCat 依赖。"}
	}
	packages := napcatLinuxPackages(packageManager)
	missing := make([]string, 0, len(packages)+1)
	for _, packageName := range packages {
		if !installed(packageName) {
			missing = append(missing, packageName)
		}
	}
	// A package database entry alone does not prove that the X server binary is
	// available on PATH. Keep this separate runtime check so the following
	// managed launch cannot fail after the UI already declared the host ready.
	if _, err := lookPath("Xvfb"); err != nil && !containsString(missing, packages[0]) {
		missing = append(missing, packages[0])
	}
	status := &linuxDependencyStatus{Supported: true, Ready: len(missing) == 0, PackageManager: packageManager, SystemAccount: currentSystemAccount(), Missing: missing}
	if !status.Ready {
		status.Hint = "安装 NapCat 前需要补齐 Linux 图形运行依赖。工作台会通过一次 sudo 授权安装固定的系统软件包，并自动继续安装。"
	}
	return status
}

func napcatLinuxPackages(packageManager string) []string {
	switch packageManager {
	case "apt":
		return []string{"xvfb", "libnss3", "libgbm1", "libglib2.0-0", "libatk1.0-0", "libatspi2.0-0", "libgtk-3-0", "libasound2"}
	case "dnf":
		return []string{"xorg-x11-server-Xvfb", "nss", "mesa-libgbm", "glib2", "atk", "at-spi2-atk", "gtk3", "alsa-lib"}
	default:
		return nil
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func currentSystemAccount() string {
	account, err := user.Current()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(account.Username)
}

func dpkgPackageInstalled(packageName string) bool {
	data, err := os.ReadFile("/var/lib/dpkg/status")
	if err != nil {
		return false
	}
	for _, paragraph := range strings.Split(string(data), "\n\n") {
		name, status := "", ""
		for _, line := range strings.Split(paragraph, "\n") {
			if value, ok := strings.CutPrefix(line, "Package: "); ok {
				name = strings.TrimSpace(value)
			}
			if value, ok := strings.CutPrefix(line, "Status: "); ok {
				status = strings.TrimSpace(value)
			}
		}
		if name == packageName && status == "install ok installed" {
			return true
		}
	}
	return false
}

func rpmPackageInstalled(packageName string) bool {
	result := exec.Command("rpm", "-q", packageName)
	return result.Run() == nil
}

func napcatAccounts(state State) ([]napcatAccount, error) {
	configs, err := findQQConfigs(state)
	if err != nil {
		return nil, err
	}
	items := make([]napcatAccount, 0, len(configs))
	for _, config := range configs {
		data, readErr := os.ReadFile(config.Path)
		if readErr != nil {
			continue
		}
		var document map[string]any
		if json.Unmarshal(data, &document) != nil {
			continue
		}
		network, _ := document["network"].(map[string]any)
		servers, _ := network["websocketServers"].([]any)
		item := napcatAccount{QQ: config.QQ}
		for _, raw := range servers {
			server, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			enabled, _ := server["enable"].(bool)
			if !enabled {
				continue
			}
			port, portErr := strconv.Atoi(fmt.Sprint(server["port"]))
			if portErr != nil || port <= 0 {
				continue
			}
			item.OneBotURL = "ws://127.0.0.1:" + strconv.Itoa(port)
			connection, dialErr := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
			if dialErr == nil {
				_ = connection.Close()
				item.OneBotReady = true
			}
			break
		}
		items = append(items, item)
	}
	return items, nil
}

// statusJSON returns the structured status as a JSON string.
func statusJSON() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	payload := collectStatus(state)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
