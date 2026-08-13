//go:build linux

package main

import (
	"archive/tar"
	"crypto/sha256"
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

type runtimeSBOM struct {
	Format        string            `json:"format"`
	Platform      string            `json:"platform"`
	RuntimeID     string            `json:"runtimeID"`
	Archive       string            `json:"archive"`
	ArchiveSHA256 string            `json:"archiveSha256"`
	Files         []runtimeSBOMFile `json:"files"`
}

type runtimeSBOMFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
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
	if err := linuxPreflightError(); err != nil {
		return linuxEnvironment{}, err
	}
	reason := linuxSystemRuntimeDiagnostic()
	if reason == "" {
		if xvfb, err := exec.LookPath("Xvfb"); err == nil && xvfb != "" {
			return linuxEnvironment{Mode: "system", Diagnostic: "已使用系统图形运行环境"}, nil
		}
	}
	// The old fallback contained Xvfb but not the complete Electron/GTK/XKB
	// runtime. Launching QQ through it merely moved a missing-library error to
	// the 85% wait. Do not claim this is portable compatibility: require the
	// reviewed system dependency operation before an Electron process starts.
	if forceManaged {
		return linuxEnvironment{}, fmt.Errorf("系统 QQ 运行依赖仍不完整：%s。请先执行“准备 QQ 登录运行环境”，完成后重新安装", reason)
	}
	return linuxEnvironment{}, fmt.Errorf("Linux 图形运行环境未就绪：%s。请先执行“准备 QQ 登录运行环境”（会安装 Xvfb、XKB、GTK、NSS、GBM、音频和 X11 依赖）", reason)
}

// linuxSystemRuntimeDiagnostic returns the first missing prerequisite of the
// native Linux graphics runtime, or "" when the system path is usable. It is
// the single source of truth for both the usability probe and user-facing
// diagnostics, so an install failure names the exact missing component.
func linuxSystemRuntimeDiagnostic() string {
	xvfb, err := exec.LookPath("Xvfb")
	if err != nil {
		return "缺少 Xvfb（虚拟显示服务器）"
	}
	if _, err := exec.LookPath("xkbcomp"); err != nil {
		return "缺少 xkbcomp（X 键盘配置工具）"
	}
	if !dirExists("/usr/share/X11/xkb") {
		return "缺少 XKB 键盘布局数据（/usr/share/X11/xkb）"
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		if _, dnfErr := exec.LookPath("dnf"); dnfErr != nil {
			return "未检测到 apt-get 或 dnf 包管理器"
		}
	}
	// Alpine and similar musl systems can expose Xvfb but cannot directly run
	// the reviewed glibc QQ package. Select the managed loader proactively.
	if matches, _ := filepath.Glob("/lib/ld-musl-*.so.1"); len(matches) > 0 {
		return "检测到 musl 运行库；官方 QQ 是 glibc 运行时"
	}
	if !linuxProgramDependenciesUsable(xvfb) {
		return "Xvfb 动态库不完整（ldd 报告缺失依赖）"
	}
	return ""
}

func linuxSystemRuntimeUsable() bool { return linuxSystemRuntimeDiagnostic() == "" }

// linuxProgramDependenciesUsable identifies the ordinary missing-library
// failure before QQ is launched. ldd is a local inspection tool here; its
// output is never executed as shell input.
func linuxProgramDependenciesUsable(program string) bool {
	output, err := exec.Command("ldd", program).CombinedOutput()
	if err != nil {
		return false
	}
	return !strings.Contains(string(output), "not found")
}

func linuxQQDependenciesUsable(program string) bool { return linuxProgramDependenciesUsable(program) }

func linuxEnvironmentForState(state State) (linuxEnvironment, error) {
	if err := linuxPreflightError(); err != nil {
		return linuxEnvironment{}, err
	}
	if state.EnvironmentMode == "managed-runtime" {
		// Older plugin versions recorded a partial managed runtime. Re-evaluate
		// the host instead of using that cache to launch Electron with an
		// incomplete dynamic-library set.
		if linuxSystemRuntimeUsable() && linuxProgramDependenciesUsable(filepath.Join(state.InstallDir, "opt", "QQ", "qq")) {
			return linuxEnvironment{Mode: "system", Diagnostic: "已使用补齐后的系统图形运行环境"}, nil
		}
		return linuxEnvironment{}, errors.New("检测到旧版兼容运行时记录，但它不包含完整 Electron 依赖；请先执行“准备 QQ 登录运行环境”，然后重新安装 NapCat")
	}
	if linuxSystemRuntimeUsable() && linuxProgramDependenciesUsable(filepath.Join(state.InstallDir, "opt", "QQ", "qq")) {
		return linuxEnvironment{Mode: "system", Diagnostic: "已使用系统图形运行环境"}, nil
	}
	return linuxEnvironment{}, errors.New("系统图形运行环境或 QQ 动态库缺失；请先执行“准备 QQ 登录运行环境”，完成后再启动")
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
	checksums, err := releaseAssetByName(release, "SHA256SUMS")
	if err != nil {
		return managedLinuxRuntime{}, fmt.Errorf("当前 QQ 插件 Release（%s）缺少 SHA256SUMS，拒绝下载兼容运行环境", release.TagName)
	}
	sbom, err := releaseAssetByName(release, assetContract.Name+".sbom.json")
	if err != nil {
		return managedLinuxRuntime{}, fmt.Errorf("当前 QQ 插件 Release（%s）缺少 %s，拒绝下载兼容运行环境", release.TagName, assetContract.Name+".sbom.json")
	}
	return installManagedLinuxRuntime(assetContract, asset, checksums, sbom)
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

func installManagedLinuxRuntime(contract linuxCompatibilityAsset, asset, checksums, sbom releaseAsset) (managedLinuxRuntime, error) {
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
	checksumFile := filepath.Join(stage, "SHA256SUMS")
	sbomFile := filepath.Join(stage, "runtime.sbom.json")
	reportNapcatProgress("runtime", 15, "正在准备兼容运行环境")
	if err := downloadFileWithProgress(asset.URL, archive, napcatDownloadProgress("下载兼容运行环境", 15, 25)); err != nil {
		return managedLinuxRuntime{}, err
	}
	if err := downloadFile(checksums.URL, checksumFile); err != nil {
		return managedLinuxRuntime{}, fmt.Errorf("下载兼容运行环境校验清单失败：%w", err)
	}
	if err := verifyReleasedAssetSHA256(archive, checksumFile, asset.Name); err != nil {
		return managedLinuxRuntime{}, err
	}
	if err := downloadFile(sbom.URL, sbomFile); err != nil {
		return managedLinuxRuntime{}, fmt.Errorf("下载兼容运行环境 SBOM 失败：%w", err)
	}
	extracted := filepath.Join(stage, "extracted")
	reportNapcatProgress("runtime", 25, "验证兼容运行环境")
	if err := extractCompatibilityRuntime(archive, extracted); err != nil {
		return managedLinuxRuntime{}, err
	}
	if err := verifyRuntimeSBOM(extracted, sbomFile, contract, asset.Name, archive); err != nil {
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

func verifyRuntimeSBOM(root, sbomPath string, contract linuxCompatibilityAsset, archiveName, archivePath string) error {
	data, err := os.ReadFile(sbomPath)
	if err != nil {
		return err
	}
	var sbom runtimeSBOM
	if err := json.Unmarshal(data, &sbom); err != nil {
		return fmt.Errorf("兼容运行环境 SBOM 无效：%w", err)
	}
	if sbom.Format != "alx-runtime-sbom/v1" || sbom.Platform != contract.Platform || sbom.RuntimeID == "" || sbom.Archive != archiveName || len(sbom.ArchiveSHA256) != sha256.Size*2 {
		return errors.New("兼容运行环境 SBOM 与当前资产不匹配")
	}
	archiveHash, err := fileSHA256(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(sbom.ArchiveSHA256, archiveHash) {
		return errors.New("兼容运行环境 SBOM 未绑定到已验证的发布资产")
	}
	expected := make(map[string]runtimeSBOMFile, len(sbom.Files))
	for _, item := range sbom.Files {
		path, pathErr := managedRuntimePath(root, filepath.FromSlash(item.Path))
		if pathErr != nil || item.Path == "" || strings.Contains(item.Path, "\\") || len(item.SHA256) != sha256.Size*2 || item.Size < 0 {
			return errors.New("兼容运行环境 SBOM 包含无效文件记录")
		}
		key, relErr := filepath.Rel(root, path)
		if relErr != nil || filepath.ToSlash(key) != item.Path {
			return errors.New("兼容运行环境 SBOM 包含非规范路径")
		}
		if _, exists := expected[item.Path]; exists {
			return errors.New("兼容运行环境 SBOM 包含重复文件记录")
		}
		expected[item.Path] = item
	}
	actual := map[string]runtimeSBOMFile{}
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("兼容运行环境包含未登记的特殊文件")
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		key := filepath.ToSlash(relative)
		hash := sha256.Sum256(contents)
		actual[key] = runtimeSBOMFile{Path: key, SHA256: fmt.Sprintf("%x", hash[:]), Size: int64(len(contents))}
		return nil
	})
	if err != nil {
		return err
	}
	if len(actual) != len(expected) {
		return errors.New("兼容运行环境文件数量与 SBOM 不一致")
	}
	for path, want := range expected {
		got, ok := actual[path]
		if !ok || got.Size != want.Size || !strings.EqualFold(got.SHA256, want.SHA256) {
			return fmt.Errorf("兼容运行环境文件校验失败：%s", path)
		}
	}
	return nil
}

func verifyReleasedAssetSHA256(archive, manifestPath, assetName string) error {
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	expected := ""
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == assetName {
			expected = fields[0]
			break
		}
	}
	if len(expected) != sha256.Size*2 {
		return fmt.Errorf("兼容运行环境校验清单未包含 %s", assetName)
	}
	actual, err := fileSHA256(archive)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("兼容运行环境 SHA-256 校验失败（%s）", assetName)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
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
