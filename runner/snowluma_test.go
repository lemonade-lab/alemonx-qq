package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withSnowLumaState(t *testing.T) string {
	t.Helper()
	original := userConfigDir
	base := t.TempDir()
	userConfigDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userConfigDir = original })
	install, err := snowLumaInstallDir()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(install, "native"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(install, "index.mjs"), []byte("// fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, native, ok := snowLumaPlatform()
	if !ok {
		t.Skip("SnowLuma is unsupported on this test platform")
	}
	if err = os.WriteFile(filepath.Join(install, "native", native), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = saveSnowLumaState(snowLumaState{InstallDir: install, Managed: true}); err != nil {
		t.Fatal(err)
	}
	return install
}

func TestSnowLumaPortsReadOfficialConfig(t *testing.T) {
	install := withSnowLumaState(t)
	if err := os.MkdirAll(filepath.Join(install, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install, "config", "runtime.json"), []byte(`{"webuiPort": 5199}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `{"networks":{"wsServers":[{"enabled":true,"port":3101,"accessToken":"ws-token"}]}}`
	if err := os.WriteFile(filepath.Join(install, "config", "onebot.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	ports := snowLumaPortsFor(install)
	if ports.WebUI != 5199 || ports.OneBot != 3101 || !ports.OneBotEnabled {
		t.Fatalf("ports = %#v, want web=5199 onebot=3101 enabled", ports)
	}
}

func TestSnowLumaOneBotTokenReadsEnabledWebSocket(t *testing.T) {
	install := withSnowLumaState(t)
	if err := os.MkdirAll(filepath.Join(install, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{"networks":{"wsServers":[{"enabled":false,"accessToken":"disabled"},{"enabled":true,"accessToken":"ws-secret"}]}}`
	if err := os.WriteFile(filepath.Join(install, "config", "onebot.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := snowLumaOneBotToken()
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err = json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["token"] != "ws-secret" {
		t.Fatalf("token = %q, want ws-secret", payload["token"])
	}
}

func TestSnowLumaOneBotTokenRejectsMissingConfig(t *testing.T) {
	withSnowLumaState(t)
	if _, err := snowLumaOneBotToken(); err == nil {
		t.Fatal("missing SnowLuma OneBot config must not silently return an empty token")
	}
}
