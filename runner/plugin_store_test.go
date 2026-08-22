package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginStoreMigratesLegacyData(t *testing.T) {
	legacy, target := filepath.Join(t.TempDir(), "legacy"), filepath.Join(t.TempDir(), "workspace", "store", "alemonx-qq")
	if err := os.MkdirAll(filepath.Join(legacy, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "runtime", "state.json"), []byte("saved"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The host creates an empty store directory before starting the runner.
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALX_PLUGIN_STORE", target)
	got, err := pluginStoreDir(legacy)
	if err != nil || got != target {
		t.Fatalf("store = %q, %v", got, err)
	}
	data, err := os.ReadFile(filepath.Join(target, "runtime", "state.json"))
	if err != nil || string(data) != "saved" {
		t.Fatalf("migrated data = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "runtime", "state.json")); err != nil {
		t.Fatalf("legacy rollback copy missing: %v", err)
	}
}
