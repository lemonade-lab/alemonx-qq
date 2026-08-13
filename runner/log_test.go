package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearNapcatLogsTruncatesCoreAndOperationLogs(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = original })

	core, err := logPath()
	if err != nil {
		t.Fatal(err)
	}
	operation, err := napcatOperationLogPath()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{core, operation} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("old log line\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output, err := clearNapcatLogs()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "已清空") {
		t.Fatalf("unexpected output: %q", output)
	}
	for _, path := range []string{core, operation} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 0 {
			t.Fatalf("%s was not cleared: %q", path, data)
		}
	}
}

func TestClearLuckyLogsIgnoresMissingFiles(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = original })

	output, err := clearLuckyLogs()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "已清空") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestTailLogAtCapsReadSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "napcat.log")
	head := strings.Repeat("a", maxLogTailBytes/2)
	tail := strings.Repeat("b", maxLogTailBytes/2+1024)
	if err := os.WriteFile(path, []byte(head+"\n"+tail), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := tailLogAt(path, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(output) > maxLogTailBytes {
		t.Fatalf("tail too large: %d > %d", len(output), maxLogTailBytes)
	}
	if !strings.HasSuffix(output, tail) {
		t.Fatal("tail does not end with the newest content")
	}
	if strings.Contains(output, "a") {
		t.Fatal("tail should drop the partial head line")
	}
}

func TestOpenAppendLogRotatesOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "napcat.log")
	if err := os.WriteFile(path, make([]byte, maxLogFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	handle, err := openAppendLog(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = handle.Close()
	if _, err := os.Stat(path + ".old"); err != nil {
		t.Fatalf("rotated copy missing: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("fresh log should start empty, got %d bytes", info.Size())
	}
}

func TestOpenAppendLogKeepsSmallFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "luckylillia.log")
	if err := os.WriteFile(path, []byte("small\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handle, err := openAppendLog(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = handle.Close()
	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Fatalf("small file must not rotate, old copy exists: %v", err)
	}
}
