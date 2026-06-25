// Package engine — общий движок управления (Go). Здесь порт generate_config из
// py_modules/config.py: загружает конфиг подписки, накладывает override.yaml
// (DNS под РФ, TUN, форс-группы GEEKCOM-VPN/GEEKCOM-AUTO, форс-правила) и пишет
// running_config для mihomo. Один источник истины для плагина и GUI.
package engine

import (
	_ "embed"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

//go:embed override.yaml
var overrideYAML []byte

// Opts повторяет параметры generate_config из плагина.
type Opts struct {
	OriPath           string // конфиг подписки (subscriptions/<current>.yaml)
	NewPath           string // куда писать running_config
	Secret            string
	OverrideDNS       bool
	EnhancedMode      string // "fake-ip" | "redir-host"
	ControllerPort    int
	AllowRemoteAccess bool
	DashboardDir      string
	Dashboard         string
	SkipSteamDownload bool
}

func GenerateConfig(o Opts) error {
	cfg, err := loadYAMLFile(o.OriPath)
	if err != nil {
		return fmt.Errorf("load subscription config: %w", err)
	}
	var ov map[string]any
	if err := yaml.Unmarshal(overrideYAML, &ov); err != nil {
		return fmt.Errorf("parse override.yaml: %w", err)
	}

	// DNS-пресет под РФ/СНГ (DoH), с учётом enhanced-mode.
	if o.OverrideDNS {
		dns := toMap(ov["dns-override"])
		mergeMap(dns, toMap(ov[o.EnhancedMode+"-dns"]))
		cfg["dns"] = dns
	}

	// Пропуск Steam-CDN-детекта (если включено).
	if o.SkipSteamDownload {
		cfg["rules"] = append(toList(ov["skip-steam-rules"]), toList(cfg["rules"])...)
	}

	// Форс-группы: GEEKCOM-VPN (select, на неё ссылаются правила) и
	// GEEKCOM-AUTO (url-test «Авто»). Префиксуем, если их ещё нет.
	groups := toList(cfg["proxy-groups"])
	existing := map[string]bool{}
	for _, g := range groups {
		if name, ok := toMap(g)["name"].(string); ok {
			existing[name] = true
		}
	}
	var prepend []any
	for _, key := range []string{"force-proxy-group", "force-auto-group"} {
		g := toMap(ov[key])
		if name, _ := g["name"].(string); name != "" && !existing[name] {
			prepend = append(prepend, g)
		}
	}
	cfg["proxy-groups"] = append(prepend, groups...)

	// Форс-правила (системные сервисы через VPN) — наивысший приоритет.
	cfg["rules"] = append(toList(ov["force-proxy-rules"]), toList(cfg["rules"])...)

	host := "127.0.0.1"
	if o.AllowRemoteAccess {
		host = "0.0.0.0"
	}
	cfg["external-controller"] = fmt.Sprintf("%s:%d", host, o.ControllerPort)
	cfg["secret"] = o.Secret
	if o.DashboardDir != "" {
		cfg["external-ui"] = o.DashboardDir
	}
	if o.Dashboard != "" {
		cfg["external-ui-name"] = o.Dashboard
	}

	cfg["tun"] = ov["tun-override"]
	mergeMap(cfg, toMap(ov["always-override"]))

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal running config: %w", err)
	}
	return os.WriteFile(o.NewPath, out, 0o644)
}

func loadYAMLFile(p string) (map[string]any, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func toMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func toList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}

func mergeMap(a, b map[string]any) {
	for k, v := range b {
		a[k] = v
	}
}
