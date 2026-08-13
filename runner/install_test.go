package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func TestSecureArchiveTargetRejectsEscapingPath(t *testing.T) {
	if _, err := secureArchiveTarget(t.TempDir(), "../../escape"); err == nil {
		t.Fatal("escaping archive path must be rejected")
	}
}

func TestLinuxQQReleaseAssetsMatchPlatformContracts(t *testing.T) {
	want := []struct {
		manager      string
		architecture string
		name         string
		kind         string
	}{
		{"apt", "amd64", "QQ_3.2.31_260710_amd64_01.deb", "deb"},
		{"apt", "arm64", "QQ_3.2.31_260710_arm64_01.deb", "deb"},
		{"dnf", "amd64", "QQ_3.2.31_260710_x86_64_01.rpm", "rpm"},
		{"dnf", "arm64", "QQ_3.2.31_260710_aarch64_01.rpm", "rpm"},
	}
	for _, expected := range want {
		asset, err := linuxQQReleaseAssetFor(expected.architecture, expected.manager)
		if err != nil || asset.Name != expected.name || asset.Kind != expected.kind || asset.URL == "" {
			t.Fatalf("%s/%s = %#v, err=%v", expected.manager, expected.architecture, asset, err)
		}
	}
	if _, err := linuxQQReleaseAssetFor("amd64", "unknown"); err == nil {
		t.Fatal("unknown package manager must be rejected")
	}
}

func TestDownloadFileReportsProgress(t *testing.T) {
	payload := bytes.Repeat([]byte("napcat"), 32*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	var updates []int64
	destination := filepath.Join(t.TempDir(), "napcat.zip")
	err := downloadFileWithProgress(server.URL, destination, func(downloaded, total int64) {
		if total != int64(len(payload)) {
			t.Fatalf("progress total = %d, want %d", total, len(payload))
		}
		updates = append(updates, downloaded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) == 0 || updates[len(updates)-1] != int64(len(payload)) {
		t.Fatalf("progress updates = %#v", updates)
	}
	data, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("downloaded payload mismatch: %v", err)
	}
}

func TestDownloadFileRejectsTruncatedContentLength(t *testing.T) {
	payload := []byte("truncated-download")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)+8))
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	err := downloadFileWithProgress(server.URL, filepath.Join(t.TempDir(), "partial.zip"), nil)
	if err == nil || !strings.Contains(err.Error(), "下载重试后仍未完成") || !strings.Contains(err.Error(), "下载内容不完整") {
		t.Fatalf("err = %v, want detailed download failure", err)
	}
}

func TestCacheOfficialNapcatAssetReusesCompletedDownload(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		_, _ = writer.Write([]byte("official archive"))
	}))
	defer server.Close()

	root := t.TempDir()
	asset := releaseAsset{Name: "NapCat.Shell.zip", URL: server.URL}
	path, reused, err := cacheOfficialNapcatAsset(root, "v4.18.18", asset, "NapCat Linux Release 包", 20, 35)
	if err != nil || reused || requests != 1 {
		t.Fatalf("first cache result path=%q reused=%v requests=%d err=%v", path, reused, requests, err)
	}
	if _, reused, err = cacheOfficialNapcatAsset(root, "v4.18.18", asset, "NapCat Linux Release 包", 20, 35); err != nil || !reused || requests != 1 {
		t.Fatalf("cached retry reused=%v requests=%d err=%v", reused, requests, err)
	}
	if _, reused, err = cacheOfficialNapcatAsset(root, "v4.18.19", asset, "NapCat Linux Release 包", 20, 35); err != nil || reused || requests != 2 {
		t.Fatalf("new release must not reuse old archive: reused=%v requests=%d err=%v", reused, requests, err)
	}
}

func TestVerifyReleaseAssetDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "asset.zip")
	if err := os.WriteFile(path, []byte("official archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	asset := releaseAsset{Name: "asset.zip", Digest: "sha256:764884ced8d4b07eac08febddb267116e3422a66ce76eb6dddb016e36d7cd286"}
	if err := verifyReleaseAssetDigest(path, asset); err != nil {
		t.Fatal(err)
	}
	asset.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	if err := verifyReleaseAssetDigest(path, asset); err == nil {
		t.Fatal("digest mismatch must be rejected")
	}
}

func TestWaitNapcatWebUIForProcessRejectsExitedProcess(t *testing.T) {
	err := waitNapcatWebUIForProcess(99999999, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "提前退出") {
		t.Fatalf("err = %v, want early exit diagnosis", err)
	}
}

func TestOfficialReleaseHTTPClientUsesHostGitHubMirror(t *testing.T) {
	var requestedURL, authorization string
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.Query().Get("url")
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer broker.Close()
	t.Setenv("ALX_PLUGIN_DOWNLOAD_BROKER", broker.URL)
	t.Setenv("ALX_PLUGIN_DOWNLOAD_TOKEN", "one-time-token")
	response, err := officialReleaseHTTPClient(5 * time.Second).Get("https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent || requestedURL != "https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest" || authorization != "Bearer one-time-token" {
		t.Fatalf("broker response=%d url=%q authorization=%q", response.StatusCode, requestedURL, authorization)
	}
}

func linuxQQTarFixture(t *testing.T) []byte {
	t.Helper()
	var tarBuffer bytes.Buffer
	writer := tar.NewWriter(&tarBuffer)
	contents := []byte(`{"main":"./application.asar/app_launcher/index.js"}`)
	if err := writer.WriteHeader(&tar.Header{Name: "opt/QQ/resources/app/package.json", Mode: 0o644, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return tarBuffer.Bytes()
}

func writeDebFixture(t *testing.T, name string, data []byte) string {
	t.Helper()
	archive := filepath.Join(t.TempDir(), "qq.deb")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("!<arch>\n"); err != nil {
		t.Fatal(err)
	}
	writeMember := func(name string, value []byte) {
		header := []byte(name)
		header = append(header, bytes.Repeat([]byte(" "), 16-len(header))...)
		header = append(header, []byte("0           0     0     100644  ")...)
		line := make([]byte, 60)
		copy(line[:16], header[:16])
		copy(line[16:28], "0           ")
		copy(line[28:34], "0     ")
		copy(line[34:40], "0     ")
		copy(line[40:48], "100644  ")
		copy(line[48:58], []byte(fmt.Sprintf("%-10d", len(value))))
		copy(line[58:], "`\n")
		if _, err := file.Write(line); err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(value); err != nil {
			t.Fatal(err)
		}
		if len(value)%2 != 0 {
			if _, err := file.Write([]byte("\n")); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeMember("debian-binary", []byte("2.0\n"))
	writeMember(name, data)
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestExtractDebQQExtractsDataTarGzip(t *testing.T) {
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	if _, err := gzipWriter.Write(linuxQQTarFixture(t)); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	archive := writeDebFixture(t, "data.tar.gz", compressed.Bytes())
	destination := t.TempDir()
	if err := extractDebQQ(archive, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "opt", "QQ", "resources", "app", "package.json")); err != nil {
		t.Fatalf("extracted package.json missing: %v", err)
	}
}

func TestValidateLinuxQQRuntimeRejectsMissingICUData(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string][]byte{
		"opt/QQ/qq":                []byte("binary"),
		"opt/QQ/resources.pak":     []byte("resources"),
		"opt/QQ/locales/zh-CN.pak": []byte("locale"),
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, content, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateLinuxQQRuntime(root); err == nil || !strings.Contains(err.Error(), "icudtl.dat") {
		t.Fatalf("missing ICU data error = %v", err)
	}
	icu := filepath.Join(root, "opt", "QQ", "icudtl.dat")
	if err := os.WriteFile(icu, bytes.Repeat([]byte("i"), 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateLinuxQQRuntime(root); err != nil {
		t.Fatalf("complete runtime rejected: %v", err)
	}
}

func TestExtractDebQQExtractsDataTarZstd(t *testing.T) {
	var compressed bytes.Buffer
	writer, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(linuxQQTarFixture(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archive := writeDebFixture(t, "data.tar.zst", compressed.Bytes())
	destination := t.TempDir()
	if err := extractDebQQ(archive, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "opt", "QQ", "resources", "app", "package.json")); err != nil {
		t.Fatalf("extracted package.json missing: %v", err)
	}
}

func TestExtractDebQQAcceptsArMemberNameWithTrailingSlash(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(linuxQQTarFixture(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := extractDebQQ(writeDebFixture(t, "data.tar.gz/", compressed.Bytes()), destination); err != nil {
		t.Fatalf("real ar member name must be accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "opt", "QQ", "resources", "app", "package.json")); err != nil {
		t.Fatal(err)
	}
}

func TestPatchLinuxQQEntrypointCreatesManagedLoader(t *testing.T) {
	root := t.TempDir()
	packagePath := filepath.Join(root, "opt", "QQ", "resources", "app", "package.json")
	if err := os.MkdirAll(filepath.Dir(packagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packagePath, []byte(`{"main": "./application.asar/app_launcher/index.js"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(root, "napcat-runtime")
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "napcat.mjs"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := patchLinuxQQEntrypoint(root, runtimeRoot, runtimeRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(packagePath + ".alx-original"); err != nil {
		t.Fatal(err)
	}
	loader, err := os.ReadFile(filepath.Join(filepath.Dir(packagePath), "loadNapCat.js"))
	if err != nil || !bytes.Contains(loader, []byte(runtimeRoot)) || !bytes.Contains(loader, []byte("napcat.mjs")) {
		t.Fatalf("managed loader = %q, err=%v", loader, err)
	}
}

func TestNapcatShellEntrypointSupportsCurrentAndLegacyLayouts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "napcat.mjs"), []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if entry, err := napcatShellEntrypoint(root); err != nil || entry != filepath.Join(root, "napcat.mjs") {
		t.Fatalf("current shell entry = %q, %v", entry, err)
	}
	if err := os.Remove(filepath.Join(root, "napcat.mjs")); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "napcat", "napcat.mjs")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if entry, err := napcatShellEntrypoint(root); err != nil || entry != legacy {
		t.Fatalf("legacy shell entry = %q, %v", entry, err)
	}
	if err := os.RemoveAll(filepath.Join(root, "napcat")); err != nil {
		t.Fatal(err)
	}
	wrapped := filepath.Join(root, "NapCat.Shell", "napcat.mjs")
	if err := os.MkdirAll(filepath.Dir(wrapped), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapped, []byte("wrapped"), 0o600); err != nil {
		t.Fatal(err)
	}
	if entry, err := napcatShellEntrypoint(root); err != nil || entry != wrapped {
		t.Fatalf("wrapped shell entry = %q, %v", entry, err)
	}
}

func TestPatchLinuxQQEntrypointUsesExtractedShellLocation(t *testing.T) {
	root, shellRoot := t.TempDir(), t.TempDir()
	packagePath := filepath.Join(root, "opt", "QQ", "resources", "app", "package.json")
	if err := os.MkdirAll(filepath.Dir(packagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packagePath, []byte(`{"main": "./application.asar/app_launcher/index.js"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shellRoot, "napcat.mjs"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := patchLinuxQQEntrypoint(root, shellRoot, shellRoot); err != nil {
		t.Fatal(err)
	}
	loader, err := os.ReadFile(filepath.Join(filepath.Dir(packagePath), "loadNapCat.js"))
	if err != nil || !bytes.Contains(loader, []byte(shellRoot)) || !bytes.Contains(loader, []byte("napcat.mjs")) {
		t.Fatalf("loader = %q, err=%v", loader, err)
	}
}

func TestPatchLinuxQQEntrypointUsesFinalPathAfterStageRename(t *testing.T) {
	parent := t.TempDir()
	stage := filepath.Join(parent, "napcat-stage")
	finalRoot := filepath.Join(parent, "Napcat")
	packagePath := filepath.Join(stage, "opt", "QQ", "resources", "app", "package.json")
	if err := os.MkdirAll(filepath.Dir(packagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packagePath, []byte(`{"main": "./application.asar/app_launcher/index.js"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "napcat.mjs"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := patchLinuxQQEntrypoint(stage, stage, finalRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, finalRoot); err != nil {
		t.Fatal(err)
	}
	loader, err := os.ReadFile(filepath.Join(finalRoot, "opt", "QQ", "resources", "app", "loadNapCat.js"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(loader, []byte(stage)) || !bytes.Contains(loader, []byte(finalRoot)) {
		t.Fatalf("loader must use final path after rename: %q", loader)
	}
}

func TestReadRPMHeaderReadsPayloadCompressor(t *testing.T) {
	store := []byte("zstd\x00")
	header := make([]byte, 16+16+len(store))
	copy(header[:4], []byte{0x8e, 0xad, 0xe8, 1})
	binary.BigEndian.PutUint32(header[8:12], 1)
	binary.BigEndian.PutUint32(header[12:16], uint32(len(store)))
	binary.BigEndian.PutUint32(header[16:20], rpmPayloadCompressorTag)
	binary.BigEndian.PutUint32(header[20:24], 6)
	binary.BigEndian.PutUint32(header[24:28], 0)
	binary.BigEndian.PutUint32(header[28:32], 1)
	copy(header[32:], store)
	compressor, err := readRPMHeader(bytes.NewReader(header))
	if err != nil || compressor != "zstd" {
		t.Fatalf("compressor = %q, err=%v", compressor, err)
	}
}

func TestExtractNewcCPIOAlignsHeaderAndFileNameTogether(t *testing.T) {
	var payload bytes.Buffer
	writeNewcEntry(t, &payload, "opt/QQ/resources/app/package.json", 0o100644, []byte(`{"name":"qq"}`))
	writeNewcEntry(t, &payload, "TRAILER!!!", 0, nil)
	destination := t.TempDir()
	if err := extractNewcCPIO(bytes.NewReader(payload.Bytes()), destination); err != nil {
		t.Fatalf("extract valid newc payload: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "opt", "QQ", "resources", "app", "package.json"))
	if err != nil || string(data) != `{"name":"qq"}` {
		t.Fatalf("extracted payload = %q, err=%v", data, err)
	}
}

func TestExtractNewcCPIOAllowsOnlyContainedRelativeSymlink(t *testing.T) {
	var payload bytes.Buffer
	writeNewcEntry(t, &payload, "opt/QQ/qq", 0o100755, []byte("qq"))
	writeNewcEntry(t, &payload, "usr/lib/.build-id/link", 0o120777, []byte("../../../opt/QQ/qq"))
	writeNewcEntry(t, &payload, "TRAILER!!!", 0, nil)
	destination := t.TempDir()
	if err := extractNewcCPIO(bytes.NewReader(payload.Bytes()), destination); err != nil {
		t.Fatalf("extract contained symlink: %v", err)
	}
	link, err := os.Readlink(filepath.Join(destination, "usr", "lib", ".build-id", "link"))
	if err != nil || link != "../../../opt/QQ/qq" {
		t.Fatalf("symlink = %q, err=%v", link, err)
	}

	payload.Reset()
	writeNewcEntry(t, &payload, "usr/lib/escape", 0o120777, []byte("../../../../outside"))
	writeNewcEntry(t, &payload, "TRAILER!!!", 0, nil)
	if err := extractNewcCPIO(bytes.NewReader(payload.Bytes()), t.TempDir()); err == nil {
		t.Fatal("escaping symlink must be rejected")
	}
}

func writeNewcEntry(t *testing.T, writer *bytes.Buffer, name string, mode int64, data []byte) {
	t.Helper()
	nameSize := int64(len(name) + 1)
	header := fmt.Sprintf("070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x", 0, mode, 0, 0, 1, 0, len(data), 0, 0, 0, 0, nameSize, 0)
	if len(header) != 110 {
		t.Fatalf("newc header length = %d", len(header))
	}
	writer.WriteString(header)
	writer.WriteString(name)
	writer.WriteByte(0)
	for padding := (4 - ((110 + nameSize) % 4)) % 4; padding > 0; padding-- {
		writer.WriteByte(0)
	}
	writer.Write(data)
	for padding := (4 - (int64(len(data)) % 4)) % 4; padding > 0; padding-- {
		writer.WriteByte(0)
	}
}

func TestWindowsNapcatCommandRejectsBatchLauncher(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "launcher.bat"), []byte("@echo off"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := windowsNapcatCommand(State{InstallDir: root}); err == nil {
		t.Fatal("batch launcher must never be executed")
	}
	if err := os.WriteFile(filepath.Join(root, "launcher.exe"), []byte("native"), 0o700); err != nil {
		t.Fatal(err)
	}
	command, err := windowsNapcatCommand(State{InstallDir: root})
	if err != nil || command.Path != filepath.Join(root, "launcher.exe") {
		t.Fatalf("native launcher = %#v, err=%v", command, err)
	}
}
