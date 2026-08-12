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

func TestLinuxQQReleaseAssetsArePinned(t *testing.T) {
	want := []struct {
		manager      string
		architecture string
		name         string
		kind         string
		sha          string
	}{
		{"apt", "amd64", "QQ_3.2.31_260710_amd64_01.deb", "deb", "02f677feb1ce01ed293a3c7761e5dd85bd79936f57dcaa4cdb53178ae30e3d6d"},
		{"apt", "arm64", "QQ_3.2.31_260710_arm64_01.deb", "deb", "ac604371f5c486acf6cbf83dd667e622ee1f487d0c8bd425627de6d68fe34974"},
		{"dnf", "amd64", "QQ_3.2.31_260710_x86_64_01.rpm", "rpm", "be897976f9481be2d224dc4e11592126a3adf71b2c395e8273cf14ea99b5519d"},
		{"dnf", "arm64", "QQ_3.2.31_260710_aarch64_01.rpm", "rpm", "0a48d0a82881ab6a6716b7f90250ecaab1305727e7b5bf2d16c9205cb0c28995"},
	}
	for _, expected := range want {
		asset, err := linuxQQReleaseAssetFor(expected.architecture, expected.manager)
		if err != nil || asset.Name != expected.name || asset.Kind != expected.kind || asset.SHA256 != expected.sha || !validSHA(asset.SHA256) {
			t.Fatalf("%s/%s = %#v, err=%v", expected.manager, expected.architecture, asset, err)
		}
	}
	if _, err := linuxQQReleaseAssetFor("amd64", "unknown"); err == nil {
		t.Fatal("unknown package manager must be rejected")
	}
}

func TestDownloadFileReportsProgressAndHash(t *testing.T) {
	payload := bytes.Repeat([]byte("napcat"), 32*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	var updates []int64
	destination := filepath.Join(t.TempDir(), "napcat.zip")
	digest, err := downloadFileWithProgress(server.URL, destination, func(downloaded, total int64) {
		if total != int64(len(payload)) {
			t.Fatalf("progress total = %d, want %d", total, len(payload))
		}
		updates = append(updates, downloaded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if digest != "d2e1fa9733407bfafd03c68c999eef9e4dcb3dd8236c02ccadd3946aa9deabe2" {
		t.Fatalf("unexpected digest: %s", digest)
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

	_, err := downloadFileWithProgress(server.URL, filepath.Join(t.TempDir(), "partial.zip"), nil)
	if err == nil || !strings.Contains(err.Error(), "下载未完成") {
		t.Fatalf("err = %v, want simple download failure", err)
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

func TestPatchLinuxQQEntrypointCreatesManagedLoader(t *testing.T) {
	root := t.TempDir()
	packagePath := filepath.Join(root, "opt", "QQ", "resources", "app", "package.json")
	if err := os.MkdirAll(filepath.Dir(packagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packagePath, []byte(`{"main": "./application.asar/app_launcher/index.js"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := patchLinuxQQEntrypoint(root, "/home/test/Napcat"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(packagePath + ".alx-original"); err != nil {
		t.Fatal(err)
	}
	loader, err := os.ReadFile(filepath.Join(filepath.Dir(packagePath), "loadNapCat.js"))
	if err != nil || !bytes.Contains(loader, []byte("/home/test/Napcat")) {
		t.Fatalf("managed loader = %q, err=%v", loader, err)
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
