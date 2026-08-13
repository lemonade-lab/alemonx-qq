//go:build linux

package main

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestLinuxCompatibilityRuntimeContract(t *testing.T) {
	for architecture, want := range map[string]string{
		"amd64": "alemonx-qq-runtime-linux-amd64-glibc.tar.zst",
		"arm64": "alemonx-qq-runtime-linux-arm64-glibc.tar.zst",
	} {
		asset, err := linuxCompatibilityAssetFor(architecture)
		if err != nil || asset.Platform != "linux-"+architecture || asset.Name != want {
			t.Fatalf("compatibility runtime %s = %#v, err=%v", architecture, asset, err)
		}
	}
	if _, err := linuxCompatibilityAssetFor("386"); err == nil {
		t.Fatal("unsupported architecture must not receive an emulated runtime")
	}
}

func TestCompatibilityRuntimeArchiveRejectsEscapingPath(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "runtime.tar.zst")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := zstd.NewWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	tarWriter := tar.NewWriter(writer)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../../escape", Mode: 0o600, Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractCompatibilityRuntime(archive, t.TempDir()); err == nil {
		t.Fatal("compatibility runtime archive traversal must be rejected")
	}
}

func TestManagedRuntimeReleaseURLFallsBackToLatestWithoutPluginTag(t *testing.T) {
	if got := linuxCompatibilityReleaseURL(""); got != linuxCompatibilityRuntimeLatestURL {
		t.Fatalf("missing tag URL = %q, want latest %q", got, linuxCompatibilityRuntimeLatestURL)
	}
	if got := linuxCompatibilityReleaseURL("v0.0.12"); got != linuxCompatibilityRuntimeReleaseBaseURL+"v0.0.12" {
		t.Fatalf("formal tag URL = %q", got)
	}
	if got := linuxCompatibilityReleaseURL("not-a-tag"); got != linuxCompatibilityRuntimeLatestURL {
		t.Fatalf("invalid tag URL = %q, want latest", got)
	}
}
