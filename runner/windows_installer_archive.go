package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// extractWindowsNapcatInstaller is platform-neutral so its archive contract
// stays covered by tests on every CI runner. It only prepares the official UI;
// it never invokes the bundled batch files.
func extractWindowsNapcatInstaller(archive string) error {
	parent := filepath.Dir(archive)
	stage, err := os.MkdirTemp(parent, ".NapCatInstaller-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	reportNapcatProgress("extract", 80, "安全解压 Windows NapCat 安装器")
	if err := unzipArchive(archive, stage); err != nil {
		return err
	}
	launcher := filepath.Join(stage, "NapCatInstaller.exe")
	info, err := os.Stat(launcher)
	if err != nil || info.IsDir() {
		return fmt.Errorf("官方 Windows 安装器缺少 NapCatInstaller.exe")
	}
	target := filepath.Join(parent, "NapCatInstaller")
	backup := target + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	if dirExists(target) {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, target); err != nil {
		if dirExists(backup) {
			_ = os.Rename(backup, target)
		}
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	return nil
}
