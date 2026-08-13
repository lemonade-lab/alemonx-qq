//go:build linux

package main

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestLinuxProgramDependenciesUsableRejectsMissingProgram(t *testing.T) {
	if linuxProgramDependenciesUsable(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("missing executable must not pass dependency preflight")
	}
}

func TestLinuxSystemRuntimeDiagnosticReportsMissingXvfb(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	reason := linuxSystemRuntimeDiagnostic()
	if !strings.Contains(reason, "Xvfb") {
		t.Fatalf("diagnostic should name the missing component when PATH lacks it: %q", reason)
	}
}

func TestVerifyRuntimeSBOM(t *testing.T) {
	root := t.TempDir()
	contents := []byte("runtime file")
	archive := filepath.Join(t.TempDir(), "runtime.tar.zst")
	archiveContents := []byte("runtime archive")
	if err := os.WriteFile(archive, archiveContents, 0o600); err != nil {
		t.Fatal(err)
	}
	archiveDigest := sha256.Sum256(archiveContents)
	if err := os.WriteFile(filepath.Join(root, "bin"), contents, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	sbom := filepath.Join(t.TempDir(), "runtime.sbom.json")
	payload := fmt.Sprintf(`{"format":"alx-runtime-sbom/v1","platform":"linux-amd64","runtimeID":"test","archive":"runtime.tar.zst","archiveSha256":"%x","files":[{"path":"bin","sha256":"%x","size":%d}]}`, archiveDigest, digest, len(contents))
	if err := os.WriteFile(sbom, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	contract := linuxCompatibilityAsset{Platform: "linux-amd64", Name: "runtime.tar.zst"}
	if err := verifyRuntimeSBOM(root, sbom, contract, "runtime.tar.zst", archive); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin"), []byte("changed"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := verifyRuntimeSBOM(root, sbom, contract, "runtime.tar.zst", archive); err == nil {
		t.Fatal("modified runtime file must be rejected")
	}
}

func TestVerifyReleasedAssetSHA256(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "runtime.tar.zst")
	payload := []byte("verified runtime")
	if err := os.WriteFile(archive, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	checksums := filepath.Join(t.TempDir(), "SHA256SUMS")
	if err := os.WriteFile(checksums, []byte(fmt.Sprintf("%x  runtime.tar.zst\n", digest)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleasedAssetSHA256(archive, checksums, "runtime.tar.zst"); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleasedAssetSHA256(archive, checksums, "missing.tar.zst"); err == nil {
		t.Fatal("missing checksum entry must be rejected")
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
