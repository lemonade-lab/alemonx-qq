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
	Engine             string                 `json:"engine"`
	Installed          bool                   `json:"installed"`
	InstallHealthy     bool                   `json:"installHealthy"`
	Running            bool                   `json:"running"`
	PortReachable      bool                   `json:"portReachable"`
	WebUIReady         bool                   `json:"webUiReady"`
	OneBotReady        bool                   `json:"oneBotReady"`
	LoginPending       bool                   `json:"loginPending"`
	Watchdog           bool                   `json:"watchdog"`
	Version            string                 `json:"version,omitempty"`
	PID                int                    `json:"pid,omitempty"`
	WebUIURL           string                 `json:"webUiUrl,omitempty"`
	OneBotURL          string                 `json:"oneBotUrl,omitempty"`
	QRCodeAvailable    bool                   `json:"qrCodeAvailable"`
	QRCodeUpdatedAt    string                 `json:"qrCodeUpdatedAt,omitempty"`
	DiagnosticHint     string                 `json:"diagnosticHint,omitempty"`
	Supported          bool                   `json:"supported"`
	Managed            bool                   `json:"managed"`
	Platform           string                 `json:"platform,omitempty"`
	InstallMode        string                 `json:"installMode,omitempty"`
	ReleaseTag         string                 `json:"releaseTag,omitempty"`
	Asset              string                 `json:"asset,omitempty"`
	ArchiveSHA256      string                 `json:"archiveSha256,omitempty"`
	Fingerprint        string                 `json:"fingerprint,omitempty"`
	ValidatedAt        string                 `json:"validatedAt,omitempty"`
	Verified           bool                   `json:"verified"`
	VerificationReason string                 `json:"verificationReason,omitempty"`
	ManagedActions     bool                   `json:"managedActions"`
	LinuxDependencies  *linuxDependencyStatus `json:"linuxDependencies,omitempty"`
	Accounts           []napcatAccount        `json:"accounts,omitempty"`
	SelectedAccount    string                 `json:"selectedAccount,omitempty"`
	UpdatedAt          string                 `json:"updatedAt"`
	Error              string                 `json:"error,omitempty"`
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
		Asset: state.Asset, ArchiveSHA256: state.ArchiveSHA256, Fingerprint: state.Fingerprint,
		ValidatedAt: state.ValidatedAt, UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if platform != nil {
		payload.Supported, payload.Platform = true, platform.Key
	}
	// Verified answers the install-time question: can this platform consume the
	// reviewed release? ManagedActions answers the stricter runtime question:
	// does this already-installed copy still match that reviewed identity?
	// Keeping them separate lets a newly validated platform offer its first
	// installation without weakening update, start, config, or uninstall gates.
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
		reasons = append(reasons, "当前受管安装未与本机平台的真实 E2E 验证证据匹配")
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
	return napcatLinuxDependenciesFor(runtime.GOOS, exec.LookPath, dpkgPackageInstalled)
}

func napcatLinuxDependenciesFor(goos string, lookPath func(string) (string, error), installed func(string) bool) *linuxDependencyStatus {
	if goos != "linux" {
		return nil
	}
	if _, err := lookPath("apt-get"); err != nil {
		return &linuxDependencyStatus{Hint: "未检测到 APT；工作台的受控依赖安装目前仅支持 Debian/Ubuntu。"}
	}
	missing := make([]string, 0, 3)
	if _, err := lookPath("xvfb-run"); err != nil {
		missing = append(missing, "xvfb")
	}
	for _, packageName := range []string{"libnss3", "libgbm1"} {
		if !installed(packageName) {
			missing = append(missing, packageName)
		}
	}
	status := &linuxDependencyStatus{Supported: true, Ready: len(missing) == 0, PackageManager: "apt", SystemAccount: currentSystemAccount(), Missing: missing}
	if !status.Ready {
		status.Hint = "安装 NapCat 前需要补齐 Linux 图形运行依赖。工作台会通过一次 sudo 授权安装固定的 APT 软件包。"
	}
	return status
}

func currentSystemAccount() string {
	account, err := user.Current()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(account.Username)
}

func dpkgPackageInstalled(packageName string) bool {
	output, err := exec.Command("dpkg-query", "-W", "-f=${db:Status-Status}", packageName).Output()
	return err == nil && strings.TrimSpace(string(output)) == "installed"
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
