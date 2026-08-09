package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withTempState(t *testing.T) string {
	t.Helper()
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = original })
	// installDir = <dir>/alx-qq/napcat
	napcat := filepath.Join(dir, "alx-qq", "napcat")
	if err := os.MkdirAll(filepath.Join(napcat, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(napcat, "package.json"), []byte(`{"name":"napcat-test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	return napcat
}

func makeManagedNapcatForConfig(t *testing.T, napcat string) {
	t.Helper()
	platform := napcatPlatform()
	if platform == nil {
		t.Skip("unsupported host platform")
	}
	fingerprint, err := napcatFingerprint(napcat)
	if err != nil {
		t.Fatal(err)
	}
	previous := napcatReleaseValidationEvidence
	t.Cleanup(func() { napcatReleaseValidationEvidence = previous })
	evidence := napcatEvidence{Platform: platform.Key, RuntimeFingerprint: fingerprint, ValidatedAt: "2026-08-10T00:00:00Z", ProcessModel: "foreground"}
	state := State{InstallDir: napcat, Managed: true, Platform: platform.Key, InstallMode: "managed", Fingerprint: fingerprint, ValidatedAt: evidence.ValidatedAt}
	if platform.Key == "windows-amd64" {
		evidence.Tag, evidence.Asset, evidence.ArchiveSHA256 = "v1.0.0", windowsAsset, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		state.ReleaseTag, state.Asset, state.ArchiveSHA256 = evidence.Tag, evidence.Asset, evidence.ArchiveSHA256
	} else {
		evidence.InstallerCommit, evidence.InstallerSHA256 = "commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		state.ReleaseTag, state.ArchiveSHA256 = evidence.InstallerCommit, evidence.InstallerSHA256
	}
	data, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	napcatReleaseValidationEvidence = base64.RawURLEncoding.EncodeToString(data)
	if err := saveState(state); err != nil {
		t.Fatal(err)
	}
}

func TestFindQQConfig(t *testing.T) {
	napcat := withTempState(t)
	// No config yet -> error.
	if _, err := findQQConfig(); err == nil {
		t.Fatal("missing config must error")
	}
	config := `{"network":{"httpServers":[{"enable":true,"port":3000,"token":"secret"}],"websocketServers":[{"enable":true,"port":3001}]}}`
	if err := os.WriteFile(filepath.Join(napcat, "config", "onebot11_12345.json"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := findQQConfig()
	if err != nil {
		t.Fatalf("findQQConfig: %v", err)
	}
	if cfg.QQ != "12345" {
		t.Fatalf("QQ = %q", cfg.QQ)
	}
}

func TestReadOnebotConfigRedactsToken(t *testing.T) {
	napcat := withTempState(t)
	config := `{"network":{"httpServers":[{"enable":true,"port":3000,"token":"supersecret"}],"websocketServers":[{"enable":false,"port":3001}]}}`
	path := filepath.Join(napcat, "config", "onebot11_1.json")
	if err := os.WriteFile(path, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	text, err := readOnebotConfig(qqConfig{QQ: "1", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if contains(text, "supersecret") {
		t.Fatalf("token leaked in output: %s", text)
	}
	if !contains(text, redactedToken) {
		t.Fatalf("token should be redacted: %s", text)
	}
}

func TestSetServerConfigPreservesOtherFields(t *testing.T) {
	napcat := withTempState(t)
	makeManagedNapcatForConfig(t, napcat)
	path := filepath.Join(napcat, "config", "onebot11_1.json")
	config := `{"network":{"httpServers":[{"enable":true,"port":3000,"token":"old","messagePostFormat":"array"}],"websocketServers":[{"enable":true,"port":3001,"heartInterval":30000}]},"enableLocalFile2Url":false}`
	if err := os.WriteFile(path, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	// Update HTTP server port + token; keep the rest intact.
	if _, err := setServerConfig(map[string]string{"port": "3456", "enable": "true", "token": "newsecret"}, false, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"messagePostFormat", "heartInterval", "enableLocalFile2Url", "3001"} {
		if !contains(text, want) {
			t.Fatalf("field %q lost after write: %s", want, text)
		}
	}
	if !contains(text, "3456") || !contains(text, "newsecret") {
		t.Fatalf("updated values missing: %s", text)
	}
}

func TestSetServerConfigRedactedTokenUnchanged(t *testing.T) {
	napcat := withTempState(t)
	makeManagedNapcatForConfig(t, napcat)
	path := filepath.Join(napcat, "config", "onebot11_1.json")
	config := `{"network":{"httpServers":[{"enable":true,"port":3000,"token":"keepme"}],"websocketServers":[]}}`
	if err := os.WriteFile(path, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	// Passing the redacted sentinel must not overwrite the real token.
	if _, err := setServerConfig(map[string]string{"port": "3456", "token": redactedToken}, false, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !contains(string(data), "keepme") {
		t.Fatalf("redacted sentinel must leave token intact: %s", data)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
