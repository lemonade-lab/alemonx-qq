package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLuckyLilliaQRCodeActionReadsKnownTempFile(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = original })

	install := filepath.Join(dir, "luckylillia")
	qrDir := filepath.Join(install, "bin", "llbot", "data", "temp")
	if err := os.MkdirAll(qrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(qrDir, "login-qrcode.png"), append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("qr")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveLuckyState(luckyState{InstallDir: install}); err != nil {
		t.Fatal(err)
	}
	output, err := luckylilliaQRCodeAction()
	if err != nil {
		t.Fatal(err)
	}
	var payload napcatQRCode
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Available || payload.Data == "" || payload.UpdatedAt == "" {
		t.Fatalf("unexpected QR payload: %+v", payload)
	}
}

func TestLuckyLilliaQRCodeAcceptsLegacyDataTempLayout(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = original })

	install := filepath.Join(dir, "luckylillia")
	if err := os.MkdirAll(filepath.Join(install, "data", "temp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install, "data", "temp", "login-qrcode.png"), append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("qr")...), 0o600); err != nil {
		t.Fatal(err)
	}
	available, _ := luckyQRCodeStatus(luckyState{InstallDir: install})
	if !available {
		t.Fatal("expected legacy data/temp layout to be recognized")
	}
}

func TestLuckyLilliaQRCodeStatusIgnoresMissingFile(t *testing.T) {
	dir := t.TempDir()
	available, updatedAt := luckyQRCodeStatus(luckyState{InstallDir: dir})
	if available || updatedAt != "" {
		t.Fatalf("unexpected status for missing QR: available=%v updatedAt=%q", available, updatedAt)
	}
}
