//go:build windows

package main

// Windows uses the official external launcher for NapCat, so no runner-owned
// install directory is mutated there.
func acquireNapcatLifecycleLock() (func(), error) { return func() {}, nil }
