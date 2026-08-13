package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const maxNapCatQRCodeSize = 2 << 20

type napcatQRCode struct {
	Available bool   `json:"available"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Data      string `json:"data,omitempty"`
}

// napcatQRCodeFile intentionally searches only the known NapCat runtime
// layout. The UI never receives this path: it gets image bytes through the
// workbench's authenticated plugin proxy.
func napcatQRCodeFile(state State) (string, os.FileInfo, error) {
	root := state.InstallDir
	if root == "" {
		var err error
		root, err = platformInstallDir()
		if err != nil {
			return "", nil, err
		}
	}
	path := filepath.Join(root, "cache", "qrcode.png")
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxNapCatQRCodeSize {
		return "", nil, errors.New("NapCat 二维码文件无效")
	}
	return path, info, nil
}

func napcatQRCodeStatus(state State) (bool, string) {
	_, info, err := napcatQRCodeFile(state)
	if err != nil || info == nil {
		return false, ""
	}
	return true, info.ModTime().UTC().Format(time.RFC3339Nano)
}

func napcatQRCodeAction() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	path, info, err := napcatQRCodeFile(state)
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
			return "", errors.New("NapCat 二维码不是有效 PNG 图片")
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
