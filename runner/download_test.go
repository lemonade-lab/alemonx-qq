package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfficialReleaseMetadataURLAppliesMirror(t *testing.T) {
	t.Setenv("ALX_PLUGIN_GITHUB_API_MIRROR", "https://api.example.local")
	got := officialReleaseMetadataURL("https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest")
	want := "https://api.example.local/repos/NapNeko/NapCatQQ/releases/latest"
	if got != want {
		t.Fatalf("mirrored URL = %q, want %q", got, want)
	}
	t.Setenv("ALX_PLUGIN_GITHUB_API_MIRROR", "")
	if got := officialReleaseMetadataURL("https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest"); got != "https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest" {
		t.Fatalf("empty mirror must keep original URL, got %q", got)
	}
}

func TestAssetCandidateURLsMirrorOnlyGitHub(t *testing.T) {
	t.Setenv("ALX_PLUGIN_DOWNLOAD_MIRRORS", " https://ghfast.top, https://gh-proxy.com ,,bad-mirror")
	githubURL := "https://github.com/acme/releases/download/v1/pkg.zip"
	candidates := assetCandidateURLs(githubURL)
	want := []string{
		githubURL,
		"https://ghfast.top/" + githubURL,
		"https://gh-proxy.com/" + githubURL,
	}
	if len(candidates) != len(want) {
		t.Fatalf("candidates = %v, want %v", candidates, want)
	}
	for index := range want {
		if candidates[index] != want[index] {
			t.Fatalf("candidate[%d] = %q, want %q", index, candidates[index], want[index])
		}
	}
	tencentURL := "https://qqdl.gtimg.cn/qqfile/QQNT/9.9.32/release/c390e792/QQ.deb"
	if candidates := assetCandidateURLs(tencentURL); len(candidates) != 1 || candidates[0] != tencentURL {
		t.Fatalf("non-GitHub URL must not be mirrored: %v", candidates)
	}
}

func TestMirrorsDisabledWhileHostBrokerConfigured(t *testing.T) {
	t.Setenv("ALX_PLUGIN_DOWNLOAD_BROKER", "https://broker.example")
	t.Setenv("ALX_PLUGIN_DOWNLOAD_TOKEN", "secret")
	t.Setenv("ALX_PLUGIN_DOWNLOAD_MIRRORS", "https://ghfast.top")
	t.Setenv("ALX_PLUGIN_GITHUB_API_MIRROR", "https://api.example.local")
	original := "https://github.com/acme/releases/download/v1/pkg.zip"
	if candidates := assetCandidateURLs(original); len(candidates) != 1 || candidates[0] != original {
		t.Fatalf("mirrors must be disabled with a broker: %v", candidates)
	}
	if got := officialReleaseMetadataURL("https://api.github.com/repos/acme/releases/latest"); got != "https://api.github.com/repos/acme/releases/latest" {
		t.Fatalf("API mirror must be disabled with a broker: %q", got)
	}
}

func TestDownloadFromCandidatesFallsBackToMirror(t *testing.T) {
	t.Setenv("ALX_PLUGIN_DOWNLOAD_MIRRORS", "https://ghfast.top")
	original := "https://github.com/acme/releases/download/v1/pkg.zip"
	dest := filepath.Join(t.TempDir(), "pkg.zip")
	once := func(url, dest string, progress downloadProgress) error {
		switch url {
		case original:
			return errors.New("direct blocked by firewall")
		case "https://ghfast.top/" + original:
			return os.WriteFile(dest, []byte("mirror-bytes"), 0o600)
		default:
			return errors.New("unexpected url: " + url)
		}
	}
	if err := downloadFromCandidates(original, dest, nil, once); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "mirror-bytes" {
		t.Fatalf("downloaded content = %q", data)
	}
}

func TestDownloadFromCandidatesReportsAllSourcesFailed(t *testing.T) {
	t.Setenv("ALX_PLUGIN_DOWNLOAD_MIRRORS", "https://ghfast.top")
	original := "https://github.com/acme/releases/download/v1/pkg.zip"
	dest := filepath.Join(t.TempDir(), "pkg.zip")
	once := func(string, string, downloadProgress) error { return errors.New("boom") }
	err := downloadFromCandidates(original, dest, nil, once)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "镜像源") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should mention mirrors and the last cause: %v", err)
	}
}
