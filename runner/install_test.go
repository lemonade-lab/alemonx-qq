package main

import (
	"strings"
	"testing"
)

func TestLinuxInstallCommandKeepsOfficialURLQuoted(t *testing.T) {
	commit := "0123456789abcdef"
	command := linuxInstallCommand(commit)
	if !strings.Contains(command, "curl -fsSL") || strings.Contains(command, "sudo") || !strings.Contains(command, linuxInstallerURL(commit)) || !strings.Contains(command, "--docker n --cli n --proxy 0") {
		t.Fatalf("unexpected terminal install command: %s", command)
	}
}
