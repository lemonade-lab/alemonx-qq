package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var onebotQQFile = regexp.MustCompile(`^onebot11_(\d+)\.json$`)

const redactedToken = "****"

type qqConfig struct {
	QQ   string `json:"qq"`
	Path string `json:"-"`
}

type onebotConfig struct {
	Network struct {
		HTTPServers      []map[string]any `json:"httpServers"`
		WebsocketServers []map[string]any `json:"websocketServers"`
	} `json:"network"`
}

func napcatConfigRoot(state State) (string, error) {
	root := state.InstallDir
	if root == "" {
		var err error
		root, err = installDir() // compatibility fallback for old local tests/state.
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(root, "config"), nil
}

func findQQConfigs(state State) ([]qqConfig, error) {
	dir, err := napcatConfigRoot(state)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("未找到 NapCat 配置；请先扫码登录 QQ 生成配置后再试")
		}
		return nil, err
	}
	items := make([]qqConfig, 0, 1)
	for _, entry := range entries {
		match := onebotQQFile.FindStringSubmatch(entry.Name())
		if len(match) == 2 && !entry.IsDir() {
			items = append(items, qqConfig{QQ: match[1], Path: filepath.Join(dir, entry.Name())})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].QQ < items[j].QQ })
	if len(items) == 0 {
		return nil, errors.New("未找到 onebot11_<QQ号>.json 配置；请先在 NapCat 管理面板完成登录并启用 OneBot")
	}
	return items, nil
}

func findQQConfigFor(state State, wanted string) (qqConfig, error) {
	items, err := findQQConfigs(state)
	if err != nil {
		return qqConfig{}, err
	}
	if wanted != "" {
		for _, item := range items {
			if item.QQ == wanted {
				return item, nil
			}
		}
		return qqConfig{}, errors.New("未找到所选 QQ 的 OneBot 配置")
	}
	if len(items) > 1 {
		return qqConfig{}, errors.New("检测到多个 QQ 配置；请选择要管理的 QQ 账号")
	}
	return items[0], nil
}

func findQQConfig() (qqConfig, error) {
	state, err := loadState()
	if err != nil {
		return qqConfig{}, err
	}
	return findQQConfigFor(state, "")
}

func readOnebotConfig(cfg qqConfig) (string, error) {
	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		return "", fmt.Errorf("无法读取配置：%w", err)
	}
	var parsed onebotConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("NapCat 配置解析失败：%w", err)
	}
	lines := []string{fmt.Sprintf("✓ OneBot 11 配置（QQ %s）：", cfg.QQ), "HTTP 服务："}
	if len(parsed.Network.HTTPServers) == 0 {
		lines = append(lines, "  （未配置）")
	}
	for i, server := range parsed.Network.HTTPServers {
		lines = append(lines, fmt.Sprintf("  #%d %s（启用：%v，端口：%v，Token：%s）", i+1, displayHost(server["host"]), server["enable"], server["port"], redact(server["token"])))
	}
	lines = append(lines, "WebSocket 服务：")
	if len(parsed.Network.WebsocketServers) == 0 {
		lines = append(lines, "  （未配置）")
	}
	for i, server := range parsed.Network.WebsocketServers {
		lines = append(lines, fmt.Sprintf("  #%d %s（启用：%v，端口：%v，Token：%s）", i+1, displayHost(server["host"]), server["enable"], server["port"], redact(server["token"])))
	}
	return strings.Join(lines, "\n"), nil
}

func onebotConfigAction(params map[string]string) (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	cfg, err := findQQConfigFor(state, param(params, "qq"))
	if err != nil {
		return "", err
	}
	return readOnebotConfig(cfg)
}

// napcatOneBotToken returns the WebSocket token currently written in the
// NapCat OneBot config for the selected QQ account. It is a read-only helper
// for the one-click sync flow; the token is never logged or stored in state.
func napcatOneBotToken(params map[string]string) (string, error) {
	state, err := loadState()
	if err != nil {
		return "", err
	}
	cfg, err := findQQConfigFor(state, param(params, "qq"))
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
	if servers, ok := network["websocketServers"].([]any); ok {
		for _, raw := range servers {
			if server, ok := raw.(map[string]any); ok {
				return oneBotTokenPayload(fmt.Sprint(server["token"]))
			}
		}
	}
	return oneBotTokenPayload("")
}

func oneBotTokenPayload(token string) (string, error) {
	data, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func displayHost(value any) string {
	if value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
		return "全部接口"
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func redact(value any) string {
	if value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
		return "（空）"
	}
	return redactedToken
}

func atomicPrivateJSON(path string, data []byte) error {
	temporary := path + ".new"
	handle, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := handle.Write(data); err != nil {
		_ = handle.Close()
		return err
	}
	if err := handle.Sync(); err != nil {
		_ = handle.Close()
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func atomicPrivateText(path, text string) error {
	temporary := path + ".new"
	if err := os.WriteFile(temporary, []byte(text), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func setServerConfig(params map[string]string, websocket, confirmed bool) (string, error) {
	if err := requireNapcatConfirmation(confirmed, "保存 OneBot 配置"); err != nil {
		return "", err
	}
	state, err := loadState()
	if err != nil {
		return "", err
	}
	if err := requireManagedNapcat(state, "写入 OneBot 配置"); err != nil {
		return "", err
	}
	cfg, err := findQQConfigFor(state, param(params, "qq"))
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
	key, label := "httpServers", "HTTP"
	if websocket {
		key, label = "websocketServers", "WebSocket"
	}
	servers, _ := network[key].([]any)
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
	server["host"] = "127.0.0.1"
	if token := param(params, "token"); token != "" && token != redactedToken {
		server["token"] = token
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := atomicPrivateJSON(cfg.Path, out); err != nil {
		return "", err
	}
	state.SelectedQQ = cfg.QQ
	if err := saveState(state); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已更新 %s 服务（QQ %s，端口 %d，监听 127.0.0.1）。\n✓ 重启 NapCat 后生效。", label, cfg.QQ, port), nil
}
