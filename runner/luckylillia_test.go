package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractLuckyZipRejectsEscapingEntry(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "invalid.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("no")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractLuckyZip(archive, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("zip slip entry must be rejected")
	}
}

func TestLuckyOneBotConfigPreservesTokenWhenRedacted(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = original })
	install, err := luckyInstallDir()
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(install, "bin", "llbot", "data", "default_config.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"ob11":{"connect":[{"type":"ws","enable":true,"port":7199,"token":"keep"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveLuckyState(luckyState{InstallDir: install}); err != nil {
		t.Fatal(err)
	}
	// Configuration writing itself is Linux ARM64-only; validate the parser and
	// token redaction path on every development platform.
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("keep")) {
		t.Fatal("fixture token missing")
	}
	text, err := luckyOneBotConfig()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains([]byte(text), []byte("keep")) {
		t.Fatalf("token leaked: %s", text)
	}
}

func TestRestoreLuckyConfigCopiesOnlyManagedFiles(t *testing.T) {
	previous, target := t.TempDir(), t.TempDir()
	source := filepath.Join(previous, "bin", "llbot", "data")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config_1.json"), []byte(`{"token":"keep"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "ignored.txt"), []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restoreLuckyConfig(previous, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "bin", "llbot", "data", "config_1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("keep")) {
		t.Fatal("config was not preserved")
	}
	if _, err := os.Stat(filepath.Join(target, "bin", "llbot", "data", "ignored.txt")); !os.IsNotExist(err) {
		t.Fatal("unmanaged file must not be copied")
	}
}

func TestLuckyReleaseAssetRequiresOfficialSHA256(t *testing.T) {
	valid := "9741b26ed6c2cbeb3bcf0fa86928f2bceeb3034a84d7950e489446936e455c1d"
	release := githubRelease{Assets: []releaseAsset{{Name: luckyAssetName, URL: "https://example.invalid/package.zip", Digest: "sha256:" + valid}}}
	asset, err := luckyReleaseAsset(release)
	if err != nil || asset.Digest != "sha256:"+valid {
		t.Fatalf("expected valid official digest, asset=%+v err=%v", asset, err)
	}
	release.Assets[0].Digest = "sha256:bad"
	if _, err := luckyReleaseAsset(release); err == nil {
		t.Fatal("asset without a valid SHA-256 digest must be rejected")
	}
}
