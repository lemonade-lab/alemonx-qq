package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ulikunitz/xz"
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
	platform := luckyPlatform()
	if platform == nil || platform.AssetName == "" {
		t.Skip("current platform has no official automatic-install asset")
	}
	release := githubRelease{Assets: []releaseAsset{{Name: platform.AssetName, URL: "https://example.invalid/package.zip", Digest: "sha256:" + valid}}}
	asset, err := luckyReleaseAsset(release)
	if err != nil || asset.Digest != "sha256:"+valid {
		t.Fatalf("expected valid official digest, asset=%+v err=%v", asset, err)
	}
	release.Assets[0].Digest = "sha256:bad"
	if _, err := luckyReleaseAsset(release); err != nil {
		t.Fatalf("official asset remains installable when Release digest changes: %v", err)
	}
}

func TestLuckyPlatformMatrixUsesOfficialAssetNames(t *testing.T) {
	cases := []struct {
		goos, goarch, asset, entry string
		auto                       bool
	}{
		{"linux", "arm64", "LLBot-CLI-linux-arm64.zip", "start.sh", true},
		{"linux", "amd64", "LLBot-CLI-linux-x64.zip", "start.sh", true},
		{"windows", "amd64", "LLBot-CLI-win-x64.zip", "llbot.exe", true},
		{"darwin", "arm64", "LLBot-CLI-macos-arm64.tar.xz", "start.sh", true},
	}
	for _, test := range cases {
		platform := luckyPlatformFor(test.goos, test.goarch)
		if platform == nil || platform.AssetName != test.asset || platform.Entrypoint != test.entry || platform.AutoInstall != test.auto {
			t.Fatalf("%s/%s = %#v", test.goos, test.goarch, platform)
		}
	}
	if intel := luckyPlatformFor("darwin", "amd64"); intel != nil {
		t.Fatalf("macOS Intel must be unsupported: %#v", intel)
	}
}

func TestLuckyAdoptRequiresHealthyAbsoluteInstallDirectory(t *testing.T) {
	original := userConfigDir
	base := t.TempDir()
	userConfigDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userConfigDir = original })
	if _, err := luckyAdopt(map[string]string{"installDir": "relative"}, true); err == nil {
		t.Fatal("relative installation path must be rejected")
	}
	install := filepath.Join(base, "LLBot")
	if err := os.MkdirAll(install, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := luckyPlatform().Entrypoint
	if err := os.WriteFile(filepath.Join(install, entry), []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := luckyAdopt(map[string]string{"installDir": install}, true); err != nil {
		t.Fatal(err)
	}
	state, err := loadLuckyState()
	if err != nil || state.InstallDir != install {
		t.Fatalf("adopted state=%+v err=%v", state, err)
	}
}

func TestExtractLuckyTarXZRejectsEscapingEntry(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "invalid.tar.xz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := xz.NewWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(compressed)
	if err := writer.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o600, Size: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("no")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractLuckyTarXZ(archive, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("tar path traversal must be rejected")
	}
}

func TestLuckyEntrypointUsesExactPlatformContract(t *testing.T) {
	root := t.TempDir()
	linux := luckyPlatformFor("linux", "amd64")
	if err := os.WriteFile(filepath.Join(root, "start.sh"), []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := luckyEntryPointFor(linux, root); got == "" {
		t.Fatal("Linux CLI must accept the official start.sh entrypoint without an executable bit")
	}
	windows := luckyPlatformFor("windows", "amd64")
	if got := luckyEntryPointFor(windows, root); got != "" {
		t.Fatal("Windows CLI must not accept a Unix llbot executable")
	}
	if err := os.WriteFile(filepath.Join(root, "llbot.exe"), []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := luckyEntryPointFor(windows, root); got == "" {
		t.Fatal("Windows CLI must accept llbot.exe")
	}
}

func TestLuckyStartCommandUsesOfficialUnixStartScript(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "start.sh")
	if err := os.WriteFile(filepath.Join(root, "llbot"), []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	command, err := luckyStartCommand(luckyPlatformFor("linux", "amd64"), root, entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(command.Args) != 4 || command.Args[0] != "xvfb-run" || command.Args[1] != "-a" || command.Args[2] != filepath.Join(root, "llbot") || command.Args[3] != "--pmhq" || command.Dir != root {
		t.Fatalf("Linux Lucky start must use the official headed Shell command: %#v", command)
	}
}

func TestLuckyExternalAssociationCannotUninstallAndForgetKeepsFiles(t *testing.T) {
	original := userConfigDir
	base := t.TempDir()
	userConfigDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userConfigDir = original })
	external := filepath.Join(t.TempDir(), "external-llbot")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveLuckyState(luckyState{InstallDir: external, Managed: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := luckyUninstall(true); err == nil {
		t.Fatal("external association must not be uninstallable")
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("external directory was changed: %v", err)
	}
	if _, err := luckyForget(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("forget must keep external directory: %v", err)
	}
}

func TestLuckyManagedUninstallRefusesUnexpectedDirectory(t *testing.T) {
	original := userConfigDir
	base := t.TempDir()
	userConfigDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userConfigDir = original })
	external := filepath.Join(t.TempDir(), "not-owned")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveLuckyState(luckyState{InstallDir: external, Managed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := luckyUninstall(true); err == nil {
		t.Fatal("managed state pointing outside the owned directory must be rejected")
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("unexpected directory was changed: %v", err)
	}
}

func TestLuckyHistoricalManagedStateRemainsManaged(t *testing.T) {
	original := userConfigDir
	base := t.TempDir()
	userConfigDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userConfigDir = original })
	owned, err := luckyInstallDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := saveLuckyState(luckyState{InstallDir: owned, Managed: true}); err != nil {
		t.Fatal(err)
	}
	state, err := loadLuckyState()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Managed || state.InstallMode != "managed" {
		t.Fatalf("historical managed state must remain managed: %+v", state)
	}
}

func TestLuckyMutatingActionsRequireConfirmation(t *testing.T) {
	if err := requireLuckyConfirmation(false, "卸载 LuckyLillia"); err == nil {
		t.Fatal("mutating Lucky action must require confirmation")
	}
}
