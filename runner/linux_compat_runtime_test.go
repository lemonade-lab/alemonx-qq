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

func TestManagedRuntimeChangesInstallationFingerprint(t *testing.T) {
	base := "5f4dcc3b5aa765d61d8327deb882cf99"
	first := linuxRuntimeStateFingerprint(base, "runtime-a")
	second := linuxRuntimeStateFingerprint(base, "runtime-b")
	if first == base || first == second {
		t.Fatalf("managed runtime must participate in state fingerprint: %q %q", first, second)
	}
}

func TestManagedRuntimeFingerprintIncludesLibraries(t *testing.T) {
	root := t.TempDir()
	for name, contents := range map[string]string{
		"alx-runtime.json":  "{}",
		"bin/Xvfb":          "xvfb",
		"lib/loader":        "loader",
		"lib/libexample.so": "first",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	first, err := managedRuntimeFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib", "libexample.so"), []byte("changed"), 0o700); err != nil {
		t.Fatal(err)
	}
	second, err := managedRuntimeFingerprint(root)
	if err != nil || first == second {
		t.Fatalf("library replacement must change runtime fingerprint: %q %q, err=%v", first, second, err)
	}
}

func TestManagedRuntimeIDCannotEscapeCacheRoot(t *testing.T) {
	for _, value := range []string{"../escape", "runtime/id", "/absolute", ""} {
		if managedRuntimeIDPattern.MatchString(value) {
			t.Fatalf("unsafe runtime ID accepted: %q", value)
		}
	}
	if !managedRuntimeIDPattern.MatchString("linux-amd64-glibc-v1") {
		t.Fatal("valid runtime ID rejected")
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
