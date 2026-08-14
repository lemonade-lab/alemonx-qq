package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var operationAction = struct {
	sync.RWMutex
	value string
}{}

func setCurrentOperationAction(action string) {
	operationAction.Lock()
	operationAction.value = action
	operationAction.Unlock()
}

func currentOperationAction() string {
	operationAction.RLock()
	defer operationAction.RUnlock()
	return operationAction.value
}

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
	handle, openErr := openAppendLog(path)
	if openErr != nil {
		return
	}
	defer handle.Close()
	_, _ = fmt.Fprintln(handle, strings.TrimRight(text, "\n"))
}

// resetActionDiagnostic starts a fresh, operation-scoped trace. It is separate
// from napcat.log, which is the long-lived child-process log and may contain
// output from previous installs.
func resetActionDiagnostic(action string) {
	path, err := actionDiagnosticLogPath(action)
	if err != nil || path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte{}, 0o600)
}

func actionDiagnosticLogPath(action string) (string, error) {
	if action == "install" || action == "update" || action == "start" || action == "restart" {
		return napcatOperationLogPath()
	}
	if strings.HasPrefix(action, "luckylillia-") {
		return luckyOperationLogPath()
	}
	if strings.HasPrefix(action, "snowluma-") {
		return snowLumaOperationLogPath()
	}
	return logPath()
}

func luckyOperationLogPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "luckylillia-operation.log"), nil
}
