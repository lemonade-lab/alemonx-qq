package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// OneBot 11 network configuration lives in config/onebot11_<QQ>.json. The
// schema can drift between NapCat releases, so the config is read and written
// generically (map[string]any) and only known fields are touched; unknown
// fields are preserved verbatim.

var onebotQQFile = regexp.MustCompile(`^onebot11_(\d+)\.json$`)

const redactedToken = "****"

type qqConfig struct {
	QQ   string
	Path string
}

// findQQConfig scans the NapCat config directory for the first OneBot config
// file and returns its QQ number and path. Requires a previous scan-login to
// have generated the per-account file.
func findQQConfig() (qqConfig, error) {
	dir, err := installDir()
	if err != nil {
		return qqConfig{}, err
	}
	configDir := filepath.Join(dir, "config")
	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return qqConfig{}, errors.New("未找到 NapCat 配置；请先扫码登录 QQ 生成配置后再试")
		}
		return qqConfig{}, err
	}
	for _, entry := range entries {
		match := onebotQQFile.FindStringSubmatch(entry.Name())
		if len(match) == 2 {
			return qqConfig{QQ: match[1], Path: filepath.Join(configDir, entry.Name())}, nil
		}
	}
	return qqConfig{}, errors.New("未找到 onebot11_<QQ号>.json 配置；请先在 NapCat 管理面板完成登录并启用 OneBot")
}

// onebotConfig is the generic shape we read/write.
type onebotConfig struct {
	Network struct {
		HTTPServers      []map[string]any `json:"httpServers"`
		WebsocketServers []map[string]any `json:"websocketServers"`
	} `json:"network"`
}

// readOnebotConfig returns a human-readable, token-redacted summary.
func readOnebotConfig(cfg qqConfig) (string, error) {
	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		return "", fmt.Errorf("无法读取配置：%w", err)
	}
	var parsed onebotConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("NapCat 配置解析失败：%w", err)
	}
	lines := []string{fmt.Sprintf("✓ OneBot 11 配置（QQ %s）：", cfg.QQ)}
	lines = append(lines, "HTTP 服务：")
	if len(parsed.Network.HTTPServers) == 0 {
		lines = append(lines, "  （未配置）")
	}
	for i, server := range parsed.Network.HTTPServers {
		lines = append(lines, fmt.Sprintf("  #%d %s（启用：%v，端口：%v，Token：%s）",
			i+1, displayHost(server["host"]), server["enable"], server["port"], redact(server["token"])))
	}
	lines = append(lines, "WebSocket 服务：")
	if len(parsed.Network.WebsocketServers) == 0 {
		lines = append(lines, "  （未配置）")
	}
	for i, server := range parsed.Network.WebsocketServers {
		lines = append(lines, fmt.Sprintf("  #%d %s（启用：%v，端口：%v，Token：%s）",
			i+1, displayHost(server["host"]), server["enable"], server["port"], redact(server["token"])))
	}
	return strings.Join(lines, "\n"), nil
}

func displayHost(value any) string {
	if value == nil {
		return "全部接口"
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return "全部接口"
	}
	return text
}

func redact(value any) string {
	if value == nil {
		return "（空）"
	}
	text := fmt.Sprint(value)
	if strings.TrimSpace(text) == "" {
		return "（空）"
	}
	return redactedToken
}

func setServerConfig(params map[string]string, websocket bool) (string, error) {
	cfg, err := findQQConfig()
	if err != nil {
		return "", err
	}
	port, err := portParam(params)
	if err != nil {
		return "", err
	}
	enable, err := boolParam(params, "enable", true)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		return "", fmt.Errorf("无法读取配置：%w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("NapCat 配置解析失败：%w", err)
	}
	network, _ := doc["network"].(map[string]any)
	if network == nil {
		network = map[string]any{}
		doc["network"] = network
	}
	key := "httpServers"
	label := "HTTP"
	if websocket {
		key = "websocketServers"
		label = "WebSocket"
	}
	servers, _ := network[key].([]any)
	if servers == nil {
		servers = []any{}
	}
	// Take the first server if present, otherwise create a fresh one.
	var server map[string]any
	if len(servers) > 0 {
		server, _ = servers[0].(map[string]any)
	}
	if server == nil {
		server = map[string]any{}
		network[key] = append(servers, server)
	}
	server["enable"] = enable
	server["port"] = port
	if host := param(params, "host"); host != "" {
		server["host"] = host
	}
	if token := param(params, "token"); token != "" && token != redactedToken {
		server["token"] = token
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(cfg.Path, out, 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已更新 %s 服务（QQ %s，端口 %d）。\n✓ 重启 NapCat 后生效。", label, cfg.QQ, port), nil
}
