package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestNapCatEvidenceRequiresExactManagedIdentity(t *testing.T) {
	platform := napcatPlatform()
	if platform == nil {
		t.Skip("unsupported host platform")
	}
	previous := napcatReleaseValidationEvidence
	t.Cleanup(func() { napcatReleaseValidationEvidence = previous })
	evidence := napcatEvidence{
		Platform:           platform.Key,
		RuntimeFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ValidatedAt:        "2026-08-10T00:00:00Z",
		ProcessModel:       "foreground",
	}
	state := State{Managed: true, InstallMode: "managed", Platform: platform.Key, Fingerprint: evidence.RuntimeFingerprint, ValidatedAt: evidence.ValidatedAt}
	if platform.Key == "windows-amd64" {
		evidence.Tag, evidence.Asset, evidence.ArchiveSHA256 = "v1.0.0", windowsAsset, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		state.ReleaseTag, state.Asset, state.ArchiveSHA256 = evidence.Tag, evidence.Asset, evidence.ArchiveSHA256
	} else {
		evidence.InstallerCommit, evidence.InstallerSHA256 = "reviewed-commit", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		state.ReleaseTag, state.ArchiveSHA256 = evidence.InstallerCommit, evidence.InstallerSHA256
	}
	data, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	napcatReleaseValidationEvidence = base64.RawURLEncoding.EncodeToString(data)
	if !napcatStateVerified(state) {
		t.Fatal("matching evidence and state must unlock managed actions")
	}
	state.Fingerprint = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if napcatStateVerified(state) {
		t.Fatal("runtime fingerprint mismatch must keep managed actions locked")
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

func TestNapcatVerificationReasonNamesCurrentPlatform(t *testing.T) {
	previous := napcatReleaseValidationEvidence
	napcatReleaseValidationEvidence = ""
	t.Cleanup(func() { napcatReleaseValidationEvidence = previous })

	platform := napcatPlatform()
	if platform == nil || !platform.AutoInstall {
		t.Skip("automatic NapCat install is unavailable on this test platform")
	}
	reason := napcatVerificationReason()
	if !strings.Contains(reason, platform.Label) || !strings.Contains(reason, "真实 E2E") {
		t.Fatalf("verification reason = %q", reason)
	}
}
