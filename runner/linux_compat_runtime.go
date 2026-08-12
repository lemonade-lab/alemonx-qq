//go:build linux

package main

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// linuxCompatibilityRuntimeReleaseURL is a separate, reviewed release line.
// The runtime archive is not a plugin bundle and never executes an installer
// script: it is a versioned collection of Xvfb, libraries and a descriptor.
// runtime-v1 is immutable in the release workflow; the asset digest returned
// by GitHub is checked before it is admitted to the cache.
const linuxCompatibilityRuntimeReleaseURL = "https://api.github.com/repos/lemonade-lab/alemonx-qq/releases/tags/runtime-v1"

const (
	linuxCompatibilityArchiveLimit = int64(300 << 20)
	linuxCompatibilityExtractLimit = int64(500 << 20)
)

var managedRuntimeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,95}$`)

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
	ID          string
	Asset       string
	SHA256      string
	Fingerprint string
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
		runtimeValue, err := loadManagedLinuxRuntime(state.RuntimeID, state.RuntimeAsset, state.RuntimeSHA256)
		if err != nil {
			return linuxEnvironment{}, fmt.Errorf("受管兼容运行环境不可用：%w", err)
		}
		return linuxEnvironment{Mode: "managed-runtime", Runtime: &runtimeValue, Reason: state.FallbackReason, Diagnostic: state.EnvironmentDiagnostic}, nil
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
	release, err := fetchRelease(linuxCompatibilityRuntimeReleaseURL, "NapCat Linux 兼容运行环境")
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	if !strings.HasPrefix(release.TagName, "runtime-v") {
		return managedLinuxRuntime{}, errors.New("兼容运行环境 Release 标签无效")
	}
	asset, err := releaseAssetByName(release, assetContract.Name)
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	digest := normalizedSHA(asset.Digest)
	if !validSHA(digest) {
		return managedLinuxRuntime{}, errors.New("兼容运行环境 Release 未提供有效 SHA-256 校验和")
	}
	return installManagedLinuxRuntime(assetContract, asset, digest)
}

func managedRuntimeBaseDir() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "runtime"), nil
}

func installManagedLinuxRuntime(contract linuxCompatibilityAsset, asset releaseAsset, digest string) (managedLinuxRuntime, error) {
	base, err := managedRuntimeBaseDir()
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return managedLinuxRuntime{}, err
	}
	if cached, err := findCachedManagedLinuxRuntime(base, asset.Name, digest); err == nil {
		reportNapcatProgress("runtime", 15, "复用已准备的兼容运行环境")
		return cached, nil
	}
	// The ID is also encoded in the archive descriptor. Until it is verified,
	// use an isolated staging directory; no unverified path reaches the cache.
	stage, err := os.MkdirTemp(base, ".download-*")
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, "package.tar.zst")
	reportNapcatProgress("runtime", 15, "正在准备兼容运行环境")
	actual, err := downloadFileLimitedWithProgress(asset.URL, archive, linuxCompatibilityArchiveLimit, napcatDownloadProgress("下载兼容运行环境", 15, 25))
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	if !strings.EqualFold(actual, digest) {
		return managedLinuxRuntime{}, errors.New("兼容运行环境 SHA-256 校验失败")
	}
	extracted := filepath.Join(stage, "extracted")
	reportNapcatProgress("runtime", 25, "验证兼容运行环境")
	if err := extractCompatibilityRuntime(archive, extracted); err != nil {
		return managedLinuxRuntime{}, err
	}
	manifest, runtimeValue, err := readManagedLinuxRuntime(extracted, contract, asset.Name, digest)
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	cacheRoot := filepath.Join(base, manifest.ID, digest)
	if cached, err := loadManagedLinuxRuntime(manifest.ID, asset.Name, digest); err == nil {
		return cached, nil
	}
	if err := os.MkdirAll(filepath.Dir(cacheRoot), 0o700); err != nil {
		return managedLinuxRuntime{}, err
	}
	if err := os.Rename(stage, cacheRoot); err != nil {
		if cached, cacheErr := loadManagedLinuxRuntime(manifest.ID, asset.Name, digest); cacheErr == nil {
			return cached, nil
		}
		return managedLinuxRuntime{}, err
	}
	// The stage moved into the cache; do not remove it via the deferred cleanup.
	stage = ""
	runtimeValue.Root = filepath.Join(cacheRoot, "extracted")
	return runtimeValue, nil
}

func findCachedManagedLinuxRuntime(base, asset, digest string) (managedLinuxRuntime, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		value, err := loadManagedLinuxRuntime(entry.Name(), asset, digest)
		if err == nil && value.ID != "" {
			return value, nil
		}
	}
	return managedLinuxRuntime{}, errors.New("未命中兼容运行环境缓存")
}

func loadManagedLinuxRuntime(id, asset, digest string) (managedLinuxRuntime, error) {
	if strings.TrimSpace(id) == "" || !validSHA(digest) || strings.TrimSpace(asset) == "" {
		return managedLinuxRuntime{}, errors.New("兼容运行环境身份不完整")
	}
	base, err := managedRuntimeBaseDir()
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	root := filepath.Join(base, id, digest, "extracted")
	contract, err := linuxCompatibilityAssetFor(runtime.GOARCH)
	if err != nil {
		return managedLinuxRuntime{}, err
	}
	_, value, err := readManagedLinuxRuntime(root, contract, asset, digest)
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
	var total int64
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
			if header.Size < 0 || total > linuxCompatibilityExtractLimit-header.Size {
				return errors.New("兼容运行环境解压后超过 500 MB 限制")
			}
			total += header.Size
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

func readManagedLinuxRuntime(root string, contract linuxCompatibilityAsset, asset, digest string) (managedRuntimeManifest, managedLinuxRuntime, error) {
	data, err := os.ReadFile(filepath.Join(root, "alx-runtime.json"))
	if err != nil {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, errors.New("兼容运行环境缺少 alx-runtime.json")
	}
	var manifest managedRuntimeManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, fmt.Errorf("兼容运行环境描述无效：%w", err)
	}
	if !managedRuntimeIDPattern.MatchString(manifest.ID) || manifest.Platform != contract.Platform || manifest.Xvfb == "" || manifest.Loader == "" || manifest.LibraryPath == "" {
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
	fingerprint, err := managedRuntimeFingerprint(root)
	if err != nil {
		return managedRuntimeManifest{}, managedLinuxRuntime{}, err
	}
	return manifest, managedLinuxRuntime{ID: manifest.ID, Asset: asset, SHA256: digest, Fingerprint: fingerprint, Root: root, Xvfb: xvfb, Loader: loader, LibraryPath: lib}, nil
}

// managedRuntimeFingerprint includes every regular file with a stable relative
// path and byte stream. An apparently valid Xvfb/loader pair is insufficient:
// a missing or replaced library must invalidate the cache and trigger a clean
// download rather than failing later during a user's login flow.
func managedRuntimeFingerprint(root string) (string, error) {
	hash := sha256.New()
	count := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("兼容运行环境包含非普通文件")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = hash.Write([]byte(filepath.ToSlash(relative)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(contents)
		count++
		return nil
	})
	if err != nil {
		return "", err
	}
	if count < 4 { // descriptor, Xvfb, loader and at least one library
		return "", errors.New("兼容运行环境内容不完整")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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

func linuxRuntimeStateFingerprint(installationFingerprint, runtimeFingerprint string) string {
	if runtimeFingerprint == "" {
		return installationFingerprint
	}
	sum := sha256.Sum256([]byte(installationFingerprint + "\x00" + runtimeFingerprint))
	return hex.EncodeToString(sum[:])
}
