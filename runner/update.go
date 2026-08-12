package main

import (
	"fmt"
	"strconv"
	"strings"
)

// versionParts splits "4.18.18" into integers for comparison.
func versionParts(version string) ([]int, error) {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if version == "" {
		return nil, fmt.Errorf("空版本号")
	}
	parts := strings.Split(version, ".")
	nums := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("版本号段 %q 不是数字", part)
		}
		nums = append(nums, n)
	}
	return nums, nil
}

// versionCompare returns -1, 0 or 1 comparing a vs b.
func versionCompare(a, b string) int {
	pa, errA := versionParts(a)
	pb, errB := versionParts(b)
	if errA != nil || errB != nil {
		return strings.Compare(a, b)
	}
	for i := 0; i < len(pa) || i < len(pb); i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

// checkUpdate compares the installed version against the latest release.
func checkUpdate() (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if !state.Managed {
		return "? 当前 NapCat 是外部关联实例；请使用其原始管理方式检查更新。", nil
	}
	if state.Platform != "windows-amd64" {
		return "✓ 当前为 Linux 受管安装。点击「更新」会下载官方 Release 并保留原配置；失败时自动回滚。", nil
	}
	release, err := fetchLatest()
	if err != nil {
		return "", err
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	lines := []string{fmt.Sprintf("✓ 当前版本：%s", state.Version), fmt.Sprintf("✓ 最新版本：%s", latest)}
	switch versionCompare(state.Version, latest) {
	case -1:
		lines = append(lines, "! 有可用更新，可在「管理」页点「更新」升级。")
	case 0:
		lines = append(lines, "✓ 已是最新版本。")
	default:
		lines = append(lines, "? 当前版本高于最新 Release（可能是开发版）。")
	}
	return strings.Join(lines, "\n"), nil
}
