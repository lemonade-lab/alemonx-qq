package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsInstallerArchiveRequiresOfficialExecutable(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "NapCat.Shell.Windows.OneKey.zip")
	writeInstallerArchive := func(entries map[string]string) {
		file, err := os.Create(archive)
		if err != nil {
			t.Fatal(err)
		}
		writer := zip.NewWriter(file)
		for name, content := range entries {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}

	writeInstallerArchive(map[string]string{"launcher.exe": "not the official entry"})
	if err := extractWindowsNapcatInstaller(archive); err == nil {
		t.Fatal("archive without NapCatInstaller.exe must be rejected")
	}
	writeInstallerArchive(map[string]string{"NapCatInstaller.exe": "official gui entry", "bootmain/napcat.bat": "never executed by ALX"})
	if err := extractWindowsNapcatInstaller(archive); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(filepath.Dir(archive), "NapCatInstaller", "NapCatInstaller.exe")
	if info, err := os.Stat(launcher); err != nil || info.IsDir() {
		t.Fatalf("extracted GUI entry missing: %v", err)
	}
}
