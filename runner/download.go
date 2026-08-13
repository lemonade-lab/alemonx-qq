package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const githubAPIBase = "https://api.github.com"

// hostDownloadBrokerConfigured reports whether the workbench download broker
// is active. The broker is the sanctioned single path; mirror fallbacks would
// only duplicate or confuse its URL handling, so they are disabled while it
// is configured.
func hostDownloadBrokerConfigured() bool {
	return strings.TrimSpace(os.Getenv("ALX_PLUGIN_DOWNLOAD_BROKER")) != "" &&
		strings.TrimSpace(os.Getenv("ALX_PLUGIN_DOWNLOAD_TOKEN")) != ""
}

// officialReleaseMetadataURL applies the optional GitHub API mirror. Common
// ghproxy deployments reject api.github.com, so this mirror is opt-in and
// operator-provided (ALX_PLUGIN_GITHUB_API_MIRROR); without it the official
// endpoint is used as-is.
func officialReleaseMetadataURL(original string) string {
	mirror := strings.TrimSpace(os.Getenv("ALX_PLUGIN_GITHUB_API_MIRROR"))
	if hostDownloadBrokerConfigured() || mirror == "" || !strings.HasPrefix(original, githubAPIBase) {
		return original
	}
	return strings.TrimRight(mirror, "/") + strings.TrimPrefix(original, githubAPIBase)
}

// downloadMirrors returns the configured GitHub download mirror prefixes
// (ALX_PLUGIN_DOWNLOAD_MIRRORS, comma-separated). A mirror is applied as
// <prefix>/<original-url>, the ghproxy convention, and only for GitHub-hosted
// files: release assets, raw content and objects.githubusercontent redirects.
func downloadMirrors() []string {
	if hostDownloadBrokerConfigured() {
		return nil
	}
	raw := strings.TrimSpace(os.Getenv("ALX_PLUGIN_DOWNLOAD_MIRRORS"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	mirrors := make([]string, 0, len(parts))
	for _, part := range parts {
		mirror := strings.TrimRight(strings.TrimSpace(part), "/")
		if mirror == "" {
			continue
		}
		if strings.HasPrefix(mirror, "http://") || strings.HasPrefix(mirror, "https://") {
			mirrors = append(mirrors, mirror)
		}
	}
	return mirrors
}

func isGitHubHostedURL(raw string) bool {
	return strings.HasPrefix(raw, "https://github.com/") ||
		strings.HasPrefix(raw, "http://github.com/") ||
		strings.HasPrefix(raw, "https://raw.githubusercontent.com/") ||
		strings.HasPrefix(raw, "https://objects.githubusercontent.com/")
}

// assetCandidateURLs lists download sources in order: the official URL first,
// then every configured mirror. Non-GitHub sources (for example the Tencent
// QQ runtime CDN) have exactly one candidate and are never proxied.
func assetCandidateURLs(raw string) []string {
	candidates := []string{raw}
	if !isGitHubHostedURL(raw) {
		return candidates
	}
	for _, mirror := range downloadMirrors() {
		candidates = append(candidates, mirror+"/"+raw)
	}
	return candidates
}

// downloadOnceFunc is the single-transfer primitive used by the candidate
// loop; keeping it a parameter makes multi-source fallback testable without
// real network access.
type downloadOnceFunc func(url, dest string, progress downloadProgress) error

func downloadFromCandidates(original, dest string, progress downloadProgress, once downloadOnceFunc) error {
	candidates := assetCandidateURLs(original)
	var lastErr error
	for index, candidate := range candidates {
		var candidateErr error
		for attempt := 0; attempt < 2; attempt++ {
			_ = os.Remove(dest)
			candidateErr = once(candidate, dest, progress)
			if candidateErr == nil {
				return nil
			}
			_ = os.Remove(dest)
		}
		lastErr = candidateErr
		if index+1 < len(candidates) {
			continue
		}
	}
	if lastErr == nil {
		lastErr = errors.New("download did not start")
	}
	if len(candidates) > 1 {
		return fmt.Errorf("直连与 %d 个镜像源均下载失败（最后错误：%w）", len(candidates)-1, lastErr)
	}
	// Keep the final transport/HTTP/I/O cause. The UI can keep its top-level
	// wording friendly, but the operation detail and core log must tell an
	// operator what actually failed after the automatic retry.
	return fmt.Errorf("下载重试后仍未完成：%w", lastErr)
}
