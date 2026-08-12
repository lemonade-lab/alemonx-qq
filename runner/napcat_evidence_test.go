package main

import "testing"

func TestExternalNapCatCannotReceiveManagedActions(t *testing.T) {
	state := State{Managed: false, InstallMode: "external"}
	if err := requireManagedNapcat(state, "卸载"); err == nil {
		t.Fatal("external association must not receive destructive permissions")
	}
	if err := requireNapcatConfirmation(false, "更新"); err == nil {
		t.Fatal("mutating action must require protocol confirmation")
	}
}
