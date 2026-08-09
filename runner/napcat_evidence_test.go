package main

import (
	"encoding/base64"
	"encoding/json"
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
