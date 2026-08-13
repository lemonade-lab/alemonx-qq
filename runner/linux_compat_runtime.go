//go:build linux

package main

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// The compatibility runtime is attached to the exact QQ plugin release that
// contains this runner. Keeping both assets in one release makes it impossible
// to publish a plugin whose automatic fallback silently points at a missing
// second tag, or have an old runner consume a newer incompatible payload.
const linuxCompatibilityRuntimeReleaseBaseURL = "https://api.github.com/repos/lemonade-lab/alemonx-qq/releases/tags/"
const linuxCompatibilityRuntimeLatestURL = "https://api.github.com/repos/lemonade-lab/alemonx-qq/releases/latest"

type linuxCompatibilityAsset struct {
	Platform string
	Name     string
}

type managedRuntimeManifest struct {
	ID          string `json:"id"`
	Platform    string `json:"platform"`
	Xvfb        string `json:"xvfb"`
	Loader      string `json:"loader"`
	LibraryPath string `json:"libraryPath"`
}

type managedLinuxRuntime struct {
	Root        string
	Xvfb        string
	Loader      string
	LibraryPath string
}

type linuxEnvironment struct {
	Mode       string
	Runtime    *managedLinuxRuntime
	Reason     string
	Diagnostic string
}

func linuxCompatibilityAssetFor(goarch string) (linuxCompatibilityAsset, error) {
	switch goarch {
	case "amd64":
		return linuxCompatibilityAsset{Platform: "linux-amd64", Name: "alemonx-qq-runtime-linux-amd64-glibc.tar.zst"}, nil
	case "arm64":
		return linuxCompatibilityAsset{Platform: "linux-arm64", Name: "alemonx-qq-runtime-linux-arm64-glibc.tar.zst"}, nil
	default:
		return linuxCompatibilityAsset{}, fmt.Errorf("Linux %s 暂不支持 NapCat 兼容运行环境", goarch)
	}
}

func prepareLinuxEnvironment(forceManaged bool) (linuxEnvironment, error) {
	if !forceManaged && linuxSystemRuntimeUsable() {
		if xvfb, err := exec.LookPath("Xvfb"); err == nil && xvfb != "" {
			return linuxEnvironment{Mode: "system", Diagnostic: "已使用系统图形运行环境"}, nil
		}
	}
	reason := "系统图形运行环境不可用"
	if forceManaged {
		reason = "系统环境准备未完成，已自动切换兼容运行环境"
	}
	runtimeValue, err := ensureManagedLinuxRuntime()
	if err != nil {
		return linuxEnvironment{}, fmt.Errorf("准备受管兼容运行环境失败：%w", err)
	}
	return linuxEnvironment{Mode: "managed-runtime", Runtime: &runtimeValue, Reason: reason, Diagnostic: "已使用受管兼容运行环境"}, nil
}

func linuxSystemRuntimeUsable() bool {
	if _, err := exec.LookPath("Xvfb"); err != nil {
		return false
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		if _, dnfErr := exec.LookPath("dnf"); dnfErr != nil {
			return false
		}
	}
	// Alpine and similar musl systems can expose Xvfb but cannot directly run
	// the reviewed glibc QQ package. Select the managed loader proactively.
	if matches, _ := filepath.Glob("/lib/ld-musl-*.so.1"); len(matches) > 0 {
		return false
	}
	return true
}

func linuxEnvironmentForState(state State) (linuxEnvironment, error) {
	if state.EnvironmentMode == "managed-runtime" {
		runtimeValue, err := loadManagedLinuxRuntime()
		if err == nil {
			return linuxEnvironment{Mode: "managed-runtime", Runtime: &runtimeValue, Reason: state.FallbackReason, Diagnostic: state.EnvironmentDiagnostic}, nil
		}
		// Cached native files can be removed by a package cleanup or a user. This
		// is recoverable: rebuild the workbench-owned runtime automatically.
		runtimeValue, refreshErr := ensureManagedLinuxRuntime()
		if refreshErr != nil {
			return linuxEnvironment{}, fmt.Errorf("兼容运行环境无法自动恢复：%w", refreshErr)
		}
		return linuxEnvironment{Mode: "managed-runtime", Runtime: &runtimeValue, Reason: "已自动恢复兼容运行环境", Diagnostic: "已使用受管兼容运行环境"}, nil
	}
	if linuxSystemRuntimeUsable() {
		return linuxEnvironment{Mode: "system", Diagnostic: "已使用系统图形运行环境"}, nil
	}
	// A system package may have been removed after install. Re-create the
	// managed fallback automatically rather than requiring a new user action.
	runtimeValue, err := ensureManagedLinuxRuntime()
	if err != nil {
		return linuxEnvironment{}, fmt.Errorf("系统图形运行环境不可用，且兼容运行环境无法准备：%w", err)
	}
	return linuxEnvironment{Mode: "managed-runtime", Runtime: &runtimeValue, Reason: "系统图形运行环境已不可用", Diagnostic: "已自动切换受管兼容运行环境"}, nil
}

func ensureManagedLinuxRuntime() (managedLinuxRuntime, error) {
	assetContract, err := linuxCompatibilityAssetFor(runtime.GOARCH)
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	releaseURL := linuxCompatibilityReleaseURL(os.Getenv("ALX_PLUGIN_INSTALLED_TAG"))
	release, err := fetchRelease(releaseURL, "NapCat Linux 兼容运行环境")
	if err != nil && releaseURL != linuxCompatibilityRuntimeLatestURL {
		release, err = fetchRelease(linuxCompatibilityRuntimeLatestURL, "NapCat Linux 兼容运行环境")
	}
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	asset, err := releaseAssetByName(release, assetContract.Name)
	if err != nil {
		return managedLinuxRuntime{}, fmt.Errorf("当前 QQ 插件 Release（%s）尚未附带 %s；请更新到包含兼容运行环境的 QQ 插件版本", release.TagName, assetContract.Name)
	}
	return installManagedLinuxRuntime(assetContract, asset)
}

// linuxCompatibilityReleaseURL accepts a formal plugin tag when available,
// while source-development and old installations transparently use latest.
func linuxCompatibilityReleaseURL(tag string) string {
	tag = strings.TrimSpace(tag)
	if strings.HasPrefix(tag, "v") && !strings.ContainsAny(tag, "/?#") {
		return linuxCompatibilityRuntimeReleaseBaseURL + tag
	}
	return linuxCompatibilityRuntimeLatestURL
}

func managedRuntimeBaseDir() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "runtime"), nil
}

func installManagedLinuxRuntime(contract linuxCompatibilityAsset, asset releaseAsset) (managedLinuxRuntime, error) {
	base, err := managedRuntimeBaseDir()
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return managedLinuxRuntime{}, err
	}
	if cached, err := loadManagedLinuxRuntime(); err == nil {
		reportNapcatProgress("runtime", 15, "复用已准备的兼容运行环境")
		return cached, nil
	}
	// Rebuild a single platform-owned runtime directory whenever its required
	// executable structure is absent. Cache identity is platform + actual
	// runnable files rather than release metadata.
	runtimeRoot := filepath.Join(base, contract.Platform)
	_ = os.RemoveAll(runtimeRoot)
	stage, err := os.MkdirTemp(base, ".download-*")
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, "package.tar.zst")
	reportNapcatProgress("runtime", 15, "正在准备兼容运行环境")
	if err := downloadFileWithProgress(asset.URL, archive, napcatDownloadProgress("下载兼容运行环境", 15, 25)); err != nil {
		return managedLinuxRuntime{}, err
	}
	extracted := filepath.Join(stage, "extracted")
	reportNapcatProgress("runtime", 25, "验证兼容运行环境")
	if err := extractCompatibilityRuntime(archive, extracted); err != nil {
		return managedLinuxRuntime{}, err
	}
	_, runtimeValue, err := readManagedLinuxRuntime(extracted, contract)
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	if err := os.Rename(stage, runtimeRoot); err != nil {
		if cached, cacheErr := loadManagedLinuxRuntime(); cacheErr == nil {
			return cached, nil
		}
		return managedLinuxRuntime{}, err
	}
	// The stage moved into the cache; do not remove it via the deferred cleanup.
	stage = ""
	runtimeValue.Root = filepath.Join(runtimeRoot, "extracted")
	return runtimeValue, nil
}

func loadManagedLinuxRuntime() (managedLinuxRuntime, error) {
	base, err := managedRuntimeBaseDir()
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	contract, err := linuxCompatibilityAssetFor(runtime.GOARCH)
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	root := filepath.Join(base, contract.Platform, "extracted")
	_, value, err := readManagedLinuxRuntime(root, contract)
	return value, err
}

func extractCompatibilityRuntime(archive, destination string) error {
	handle, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer handle.Close()
	reader, err := zstd.NewReader(handle)
	if err != nil {
		return fmt.Errorf("兼容运行环境压缩包格式无效：%w", err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := secureArchiveTarget(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return errors.New("兼容运行环境包含无效文件大小")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			handle, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode()|0o600)
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(handle, io.LimitReader(tarReader, header.Size))
			closeErr := handle.Close()
			if copyErr != nil {
				return copyErr
			}
			if written != header.Size {
				return errors.New("兼容运行环境内容不完整")
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("兼容运行环境包含不受支持的链接或特殊文件")
		}
	}
}

func readManagedLinuxRuntime(root string, contract linuxCompatibilityAsset) (managedRuntimeManifest, managedLinuxRuntime, error) {
	data, err := os.ReadFile(filepath.Join(root, "alx-runtime.json"))
	if err != nil {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, errors.New("兼容运行环境缺少 alx-runtime.json")
	}
	var manifest managedRuntimeManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, fmt.Errorf("兼容运行环境描述无效：%w", err)
	}
	if manifest.Platform != contract.Platform || manifest.Xvfb == "" || manifest.Loader == "" || manifest.LibraryPath == "" {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, errors.New("兼容运行环境描述不完整或与当前平台不匹配")
	}
	xvfb, err := managedRuntimePath(root, manifest.Xvfb)
	if err != nil {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, err
	}
	loader, err := managedRuntimePath(root, manifest.Loader)
	if err != nil {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, err
	}
	lib, err := managedRuntimePath(root, manifest.LibraryPath)
	if err != nil {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, err
	}
	if info, err := os.Stat(xvfb); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, errors.New("兼容运行环境缺少可执行 Xvfb")
	}
	if info, err := os.Stat(loader); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, errors.New("兼容运行环境缺少可执行动态加载器")
	}
	if info, err := os.Stat(lib); err != nil || !info.IsDir() {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, errors.New("兼容运行环境缺少动态库目录")
	}
	return manifest, managedLinuxRuntime{Root: root, Xvfb: xvfb, Loader: loader, LibraryPath: lib}, nil
}

func managedRuntimePath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", errors.New("兼容运行环境包含绝对路径")
	}
	path := filepath.Join(root, filepath.Clean(relative))
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(root)+string(filepath.Separator)) {
		return "", errors.New("兼容运行环境包含越界路径")
	}
	return path, nil
}

func managedRuntimeCommand(value managedLinuxRuntime, program string, args ...string) *exec.Cmd {
	arguments := []string{"--library-path", value.LibraryPath, program}
	arguments = append(arguments, args...)
	return exec.Command(value.Loader, arguments...)
}
