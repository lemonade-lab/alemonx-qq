package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxLogTailBytes caps how much of a log file is read into memory. A runaway
// core log must not turn a status poll or a log request into a multi-GB read.
const maxLogTailBytes = 1 << 20

// maxLogFileBytes triggers a rotation before appending: the current file is
// renamed to <name>.old and a fresh file is started, so long-running cores
// cannot grow a single log file without bound.
const maxLogFileBytes = 32 << 20

// openAppendLog opens a managed log for appending, rotating the file first
// when it has grown beyond maxLogFileBytes. Rotation keeps at most one
// previous copy; a missing directory is created like the callers did before.
func openAppendLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() > maxLogFileBytes {
		previous := path + ".old"
		_ = os.Remove(previous)
		_ = os.Rename(path, previous)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

// tailLog reads the last n lines of the NapCat log file.
func tailLog(lines int) (string, error) {
	path, err := logPath()
	if err != nil {
		return "", err
	}
	return tailLogAt(path, lines)
}

func tailLogAt(path string, lines int) (string, error) {
	handle, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "日志文件尚不存在；启动 NapCat 后生成。", nil
		}
		return "", err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return "", err
	}
	start := int64(0)
	if info.Size() > maxLogTailBytes {
		start = info.Size() - maxLogTailBytes
	}
	data := make([]byte, info.Size()-start)
	if _, err := handle.ReadAt(data, start); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	text := strings.TrimRight(string(data), "\n")
	// Reading from the middle of a file leaves a partial first line; drop it.
	if start > 0 {
		if index := strings.IndexByte(text, '\n'); index >= 0 {
			text = text[index+1:]
		}
	}
	if text == "" {
		return "（日志为空）", nil
	}
	parts := strings.Split(text, "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n"), nil
}

// clearLogAt truncates one managed log file. Missing files are a valid empty
// state and are reported as already-clean rather than an error.
func clearLogAt(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	return os.Truncate(path, 0)
}

func clearNapcatLogs() (string, error) {
	core, err := logPath()
	if err != nil {
		return "", err
	}
	operation, err := napcatOperationLogPath()
	if err != nil {
		return "", err
	}
	if err := clearLogAt(core); err != nil {
		return "", err
	}
	if err := clearLogAt(operation); err != nil {
		return "", err
	}
	return "✓ NapCat 日志已清空（核心日志与操作日志）。", nil
}

func clearLuckyLogs() (string, error) {
	core, err := luckyLogPath()
	if err != nil {
		return "", err
	}
	operation, err := luckyOperationLogPath()
	if err != nil {
		return "", err
	}
	if err := clearLogAt(core); err != nil {
		return "", err
	}
	if err := clearLogAt(operation); err != nil {
		return "", err
	}
	return "✓ LuckyLillia 日志已清空（核心日志与操作日志）。", nil
}
