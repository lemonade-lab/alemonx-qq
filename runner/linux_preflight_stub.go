//go:build !linux

package main

type linuxHostCompatibility struct {
	Distribution    string
	Version         string
	PackageManager  string
	Libc            string
	Container       bool
	NativeSupported bool
	Diagnostic      string
}

func currentLinuxHostCompatibility() linuxHostCompatibility { return linuxHostCompatibility{} }
func linuxPreflightError() error                            { return nil }
