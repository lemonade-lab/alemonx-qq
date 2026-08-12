package main

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func evidenceFixture(core, platform, asset, archiveSHA, fingerprint string, websocket bool) map[string]any {
	return map[string]any{
		"core": core, "platform": platform, "tag": "v1.0.0", "asset": asset,
		"archiveSha256": archiveSHA, "runtimeFingerprint": fingerprint,
		"validatedAt": "2026-08-10T00:00:00Z", "processModel": "foreground",
		"websocketRequired": websocket, "status": "passed",
	}
}

func TestPrepareManifestUsesPlatformEvidenceForEachCore(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "alx.json")
	if err := writeJSON(manifest, map[string]any{"services": []any{map[string]any{"id": "napcat-webui"}, map[string]any{"id": "luckylillia-webui"}}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "evidence", "napcat", "windows-amd64.json")
	if err := writeJSON(path, evidenceFixture("napcat", "windows-amd64", "NapCat.Shell.Windows.OneKey.zip", strings.Repeat("a", 64), strings.Repeat("b", 64), true)); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "bundle.json")
	if err := prepareManifest(root, "windows-amd64", manifest, output); err != nil {
		t.Fatal(err)
	}
	value, err := readJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	services := value["services"].([]any)
	if services[0].(map[string]any)["websocket"] != true || services[1].(map[string]any)["websocket"] != false {
		t.Fatalf("unexpected service websocket flags: %#v", services)
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

func TestValidateRuntimeRecordsRejectsScriptlessMismatch(t *testing.T) {
	record := evidenceFixture("luckylillia", "linux-amd64", luckyAssets["linux-amd64"], strings.Repeat("a", 64), strings.Repeat("b", 64), false)
	if err := validateRuntimeRecords("luckylillia", []map[string]any{record}); err != nil {
		t.Fatal(err)
	}
	record["asset"] = "start.sh"
	if err := validateRuntimeRecords("luckylillia", []map[string]any{record}); err == nil {
		t.Fatal("script asset must be rejected")
	}
}

func TestValidateNapcatLinuxEvidenceRequiresReviewedRuntime(t *testing.T) {
	record := evidenceFixture("napcat", "linux-amd64", "NapCat.Shell.zip", strings.Repeat("a", 64), strings.Repeat("b", 64), false)
	record["runtimeAsset"] = "QQ_3.2.31_260710_amd64_01.deb"
	record["runtimeArchiveSha256"] = "02f677feb1ce01ed293a3c7761e5dd85bd79936f57dcaa4cdb53178ae30e3d6d"
	if err := validateRuntimeRecords("napcat", []map[string]any{record}); err != nil {
		t.Fatal(err)
	}
	delete(record, "runtimeArchiveSha256")
	if err := validateRuntimeRecords("napcat", []map[string]any{record}); err == nil {
		t.Fatal("Linux evidence without QQ runtime SHA must be rejected")
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
