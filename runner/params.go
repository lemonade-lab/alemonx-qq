package main

import (
	"fmt"
	"strconv"
	"strings"
)

// All user-supplied values are re-validated here before any command runs.

func param(params map[string]string, key string) string {
	return strings.TrimSpace(params[key])
}

func linesParam(params map[string]string) (int, error) {
	value := param(params, "lines")
	if value == "" {
		return 200, nil
	}
	lines, err := strconv.Atoi(value)
	if err != nil || lines < 1 || lines > 5000 {
		return 0, fmt.Errorf("日志行数必须是 1 到 5000 的整数")
	}
	return lines, nil
}

func portParam(params map[string]string) (int, error) {
	value := param(params, "port")
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口必须是 1 到 65535 的整数")
	}
	return port, nil
}

func boolParam(params map[string]string, key string, fallback bool) (bool, error) {
	value := strings.ToLower(param(params, key))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s 必须是 true 或 false", key)
	}
}
