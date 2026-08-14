package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestSnowLumaAssetSelectionAndArchiveInspection(t *testing.T) {
	asset, native, err := snowLumaAssetForE2E(release{Assets: []releaseAsset{
		{Name: "SnowLuma-v1.0.0-linux-x64-lite.tar.gz"},
		{Name: "SnowLuma-v1.0.0-linux-x64.tar.gz"},
	}}, "linux-amd64")
	if err != nil || asset.Name != "SnowLuma-v1.0.0-linux-x64.tar.gz" || native != "snowluma-linux-x64.node" {
		t.Fatalf("asset=%#v native=%q err=%v", asset, native, err)
	}
	archive := filepath.Join(t.TempDir(), "snowluma.tar.gz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, name := range []string{"SnowLuma/index.mjs", "SnowLuma/native/snowluma-linux-x64.node"} {
		if err = tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err = tarWriter.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err = tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err = gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = inspectSnowLumaArchive(archive, asset.Name, native); err != nil {
		t.Fatal(err)
	}
}

func TestSnowLumaArchiveInspectionRejectsMissingNativeAddon(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "snowluma.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("SnowLuma/index.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = inspectSnowLumaArchive(archive, "SnowLuma-v1.0.0-win-x64.zip", "snowluma-win32-x64.node"); err == nil {
		t.Fatal("archive without the architecture-native addon must be rejected")
	}
}

func TestSetVersionAcceptsZeroMajorTag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alx.json")
	if err := writeJSON(path, map[string]any{"id": "alemonx-qq", "version": "0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if err := setVersion(path, "v0.0.2"); err != nil {
		t.Fatal(err)
	}
	if err := verifyVersion(path, "v0.0.2"); err != nil {
		t.Fatal(err)
	}
}

func TestPackageNapcatRuntimeCreatesArchiveChecksumAndSBOM(t *testing.T) {
	stage, output := filepath.Join(t.TempDir(), "stage"), filepath.Join(t.TempDir(), "release")
	for path, contents := range map[string]string{
		"alx-runtime.json":         `{"id":"linux-amd64-glibc-v1","platform":"linux-amd64","xvfb":"bin/Xvfb","loader":"lib/ld-linux-x86-64.so.2","libraryPath":"lib"}`,
		"bin/Xvfb":                 "xvfb",
		"lib/ld-linux-x86-64.so.2": "loader",
		"lib/libX11.so":            "library",
	} {
		full := filepath.Join(stage, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := packageNapcatRuntime([]string{"linux-amd64", stage, output}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(output, "alemonx-qq-runtime-linux-amd64-glibc.tar.zst")
	if _, err := os.Stat(archive); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(archive + ".sha256"); err != nil || !strings.Contains(string(data), filepath.Base(archive)) {
		t.Fatalf("checksum file = %q, err=%v", data, err)
	}
	if _, err := os.Stat(archive + ".sbom.json"); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := zstd.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	foundManifest := false
	for {
		header, err := tarReader.Next()
		if err != nil {
			break
		}
		if header.Name == "alx-runtime.json" {
			foundManifest = true
		}
	}
	if !foundManifest {
		t.Fatal("runtime archive lacks manifest")
	}
}
