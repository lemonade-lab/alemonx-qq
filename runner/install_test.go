package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxInstallerURLPinsCommit(t *testing.T) {
	if got, want := linuxInstallerURL("0123456789abcdef"), "https://raw.githubusercontent.com/NapNeko/NapCat-Installer/0123456789abcdef/script/install.sh"; got != want {
		t.Fatalf("installer URL = %q, want %q", got, want)
	}
}

func TestRejectPrivilegedInstallerBeforeExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncommand sudo apt-get install -y xvfb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rejectPrivilegedInstaller(path); err == nil {
		t.Fatal("installer containing sudo must be rejected")
	}
}

func TestRejectPrivilegedInstallerAllowsRootlessScript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nmkdir -p \"$HOME/Napcat\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rejectPrivilegedInstaller(path); err != nil {
		t.Fatalf("rootless installer rejected: %v", err)
	}
}
