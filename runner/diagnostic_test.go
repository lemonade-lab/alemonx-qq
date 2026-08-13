package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRecordActionFailureWritesCoreSpecificDiagnostic(t *testing.T) {
	original := userConfigDir
	base := t.TempDir()
	userConfigDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userConfigDir = original })

	recordActionFailure("luckylillia-install", errors.New("底层启动程序不存在"))
	path, err := luckyOperationLogPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "action=luckylillia-install") || !strings.Contains(string(data), "底层启动程序不存在") {
		t.Fatalf("diagnostic log = %q, err=%v", data, err)
	}
}
