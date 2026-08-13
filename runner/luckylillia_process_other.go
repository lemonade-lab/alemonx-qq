//go:build !linux

package main

import (
	"os"
)

func startLuckyProcess(platform *luckyPlatformSpec, root, entry string, log *os.File) (luckyProcess, error) {
	return startLuckyProcessDefault(platform, root, entry, log)
}
