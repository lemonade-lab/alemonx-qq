//go:build windows

package main

func managedNapcatGroupAlive(state State) bool { return processAlive(napcatProcessGroup(state)) }
