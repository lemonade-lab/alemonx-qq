//go:build !linux

package main

// installLinuxNapCat is compiled into every runner target, but only invoked on
// Linux. Keep its platform-specific preflight callable during cross-compiles.
func linuxQQDependenciesUsable(string) bool { return true }
