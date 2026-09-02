package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
)

// Tencent publishes the current Linux package locations in this public
// configuration. QQ release directories are short-lived, so a hard-coded CDN
// path inevitably turns into a 404 after Tencent rotates a release.
const tencentQQPCConfigURL = "https://qq-web.cdn-go.cn/im.qq.com_new/latest/rainbow/pcConfig.json"

type tencentQQPCConfig struct {
	Linux struct {
		X64DownloadURL map[string]string `json:"x64DownloadUrl"`
		ARMDownloadURL map[string]string `json:"armDownloadUrl"`
	} `json:"Linux"`
}

// fetchCurrentLinuxQQAsset resolves one of Tencent's currently advertised
// archives. The installer only extracts DEB/RPM payloads, and validates both
// the CDN host and filename architecture before accepting the metadata.
func fetchCurrentLinuxQQAsset(client *http.Client, goarch, preferredKind string) (linuxQQAsset, error) {
	return fetchLinuxQQAssetFromConfigURL(client, tencentQQPCConfigURL, goarch, preferredKind)
}

func fetchLinuxQQAssetFromConfigURL(client *http.Client, configURL, goarch, preferredKind string) (linuxQQAsset, error) {
	response, err := client.Get(configURL)
	if err != nil {
		return linuxQQAsset{}, fmt.Errorf("无法获取腾讯 Linux QQ 下载配置：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return linuxQQAsset{}, fmt.Errorf("腾讯 Linux QQ 下载配置请求失败（%s）", response.Status)
	}
	var config tencentQQPCConfig
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&config); err != nil {
		return linuxQQAsset{}, fmt.Errorf("腾讯 Linux QQ 下载配置解析失败：%w", err)
	}

	var sources map[string]string
	switch goarch {
	case "amd64":
		sources = config.Linux.X64DownloadURL
	case "arm64":
		sources = config.Linux.ARMDownloadURL
	default:
		return linuxQQAsset{}, fmt.Errorf("Linux %s 暂不支持自动安装 QQ 运行时", goarch)
	}
	for _, kind := range uniqueRuntimeKinds(preferredKind, "deb", "rpm") {
		raw := strings.TrimSpace(sources[kind])
		if raw == "" {
			continue
		}
		asset, err := validateTencentQQAsset(raw, goarch, kind)
		if err == nil {
			return asset, nil
		}
	}
	return linuxQQAsset{}, fmt.Errorf("腾讯当前发布配置未提供匹配 Linux %s 的 DEB/RPM 包", goarch)
}

func uniqueRuntimeKinds(values ...string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if (value != "deb" && value != "rpm") || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func validateTencentQQAsset(raw, goarch, kind string) (linuxQQAsset, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "qqdl.gtimg.cn") {
		return linuxQQAsset{}, fmt.Errorf("腾讯 Linux QQ 下载地址无效")
	}
	name := path.Base(parsed.Path)
	if !strings.HasSuffix(strings.ToLower(name), "."+kind) {
		return linuxQQAsset{}, fmt.Errorf("腾讯 Linux QQ 包格式不匹配")
	}
	lower := strings.ToLower(name)
	switch goarch {
	case "amd64":
		if !strings.Contains(lower, "amd64") && !strings.Contains(lower, "x86_64") {
			return linuxQQAsset{}, fmt.Errorf("腾讯 Linux QQ 包架构不匹配")
		}
	case "arm64":
		if !strings.Contains(lower, "arm64") && !strings.Contains(lower, "aarch64") {
			return linuxQQAsset{}, fmt.Errorf("腾讯 Linux QQ 包架构不匹配")
		}
	}
	return linuxQQAsset{Name: name, URL: raw, Kind: kind}, nil
}

func currentLinuxQQReleaseAsset(preferredKind string) (linuxQQAsset, error) {
	return fetchCurrentLinuxQQAsset(officialReleaseHTTPClient(metadataTimeout), runtime.GOARCH, preferredKind)
}
