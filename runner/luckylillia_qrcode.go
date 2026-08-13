package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// luckyQRCodeCandidates lists the known LLBot temp locations for the login QR
// PNG. The official CLI writes the code next to its private data directory
// (bin/llbot/data) whenever a fresh code is fetched; a couple of layouts are
// tolerated so a process-cwd change does not hide a valid code.
func luckyQRCodeCandidates(root string) []string {
	dirs := []string{
		filepath.Join(root, "bin", "llbot", "data", "temp"),
		filepath.Join(root, "data", "temp"),
	}
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		paths = append(paths, filepath.Join(dir, "login-qrcode.png"))
	}
	return paths
}

// luckyQRCodeFile intentionally searches only the known LLBot runtime layout.
// The UI never receives this path: it gets image bytes through the workbench's
// authenticated plugin proxy.
func luckyQRCodeFile(state luckyState) (string, os.FileInfo, error) {
	root := state.InstallDir
	if root == "" {
		var err error
		root, err = luckyInstallDir()
		if err != nil {
			return "", nil, err
		}
	}
	for _, path := range luckyQRCodeCandidates(root) {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxNapCatQRCodeSize {
			return "", nil, errors.New("LuckyLillia 二维码文件无效")
		}
		return path, info, nil
	}
	return "", nil, nil
}

func luckyQRCodeStatus(state luckyState) (bool, string) {
	_, info, err := luckyQRCodeFile(state)
	if err != nil || info == nil {
		return false, ""
	}
	return true, info.ModTime().UTC().Format(time.RFC3339Nano)
}

func luckylilliaQRCodeAction() (string, error) {
	state, err := loadLuckyState()
	if err != nil {
		return "", err
	}
	path, info, err := luckyQRCodeFile(state)
	if err != nil {
		return "", err
	}
	payload := napcatQRCode{Available: path != ""}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
			return "", errors.New("LuckyLillia 二维码不是有效 PNG 图片")
		}
		payload.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
		payload.Data = base64.StdEncoding.EncodeToString(data)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
