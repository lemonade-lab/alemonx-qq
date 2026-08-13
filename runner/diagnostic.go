package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// recordActionFailure gives every failed lifecycle action a durable diagnostic
// trail. Most install failures happen before an external process exists, so
// relying only on the child-process log would otherwise lose the useful cause.
// The UI presents a short result first and exposes this log on demand.
func recordActionFailure(action string, err error) {
	if err == nil || action == "status" || action == "napcat-status" || action == "luckylillia-status" {
		return
	}
	appendActionDiagnostic(action, fmt.Sprintf("[%s] action=%s failed\n%s\n", time.Now().UTC().Format(time.RFC3339), action, strings.TrimSpace(err.Error())))
}

// appendActionDiagnostic keeps lifecycle activity available while a long task
// is still running. It deliberately records only runner-owned stage text,
// never request parameters such as OneBot tokens.
func appendActionDiagnostic(action, text string) {
	path, pathErr := actionDiagnosticLogPath(action)
	if pathErr != nil || path == "" {
		return
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o755); mkdirErr != nil {
		return
	}
	handle, openErr := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if openErr != nil {
		return
	}
	defer handle.Close()
	_, _ = fmt.Fprintln(handle, strings.TrimRight(text, "\n"))
}

func actionDiagnosticLogPath(action string) (string, error) {
	if strings.HasPrefix(action, "luckylillia-") {
		return luckyLogPath()
	}
	return logPath()
}
