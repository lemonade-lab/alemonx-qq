package main

import "testing"

func TestNapCatAutomaticSupportDoesNotRequireReleaseEvidence(t *testing.T) {
	platform := napcatPlatform()
	if platform == nil {
		t.Skip("unsupported host platform")
	}
	previous := napcatReleaseValidationEvidence
	t.Cleanup(func() { napcatReleaseValidationEvidence = previous })
	napcatReleaseValidationEvidence = ""
	if got, want := napcatVerified(), platform.AutoInstall; got != want {
		t.Fatalf("automatic support = %v, want %v", got, want)
	}
}

func TestExternalNapCatCannotReceiveManagedActions(t *testing.T) {
	state := State{Managed: false, InstallMode: "external"}
	if err := requireManagedNapcat(state, "卸载"); err == nil {
		t.Fatal("external association must not receive destructive permissions")
	}
	if err := requireNapcatConfirmation(false, "更新"); err == nil {
		t.Fatal("mutating action must require protocol confirmation")
	}
}
