package main

import (
	"os"
	"strings"
)

// tailLog reads the last n lines of the NapCat log file.
func tailLog(lines int) (string, error) {
	path, err := logPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "日志文件尚不存在；启动 NapCat 后生成。", nil
		}
		return "", err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return "（日志为空）", nil
	}
	parts := strings.Split(text, "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n"), nil
}
