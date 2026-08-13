package main

import "fmt"

// withNapcatLifecycleLock serializes mutating NapCat actions across runner
// processes. The workbench normally serializes tasks, but the watchdog is an
// independent runner and a second client can otherwise race directory swaps.
func withNapcatLifecycleLock(action func() (string, error)) (string, error) {
	unlock, err := acquireNapcatLifecycleLock()
	if err != nil {
		return "", fmt.Errorf("NapCat 正在执行另一项生命周期操作，请完成后重试：%w", err)
	}
	defer unlock()
	return action()
}
