// Package config читает общий config.json (top-level схема, унифицированная в
// 1.2.3 — плагин и десктоп используют один слой). Пока read-only: движок-каркас
// только генерит running_config. Мутации (add-sub/set-node) с round-trip
// сохранением неизвестных ключей придут на следующем шаге.
package config

import (
	"encoding/json"
	"os"

	"geekcom-clash/internal/paths"
)

type Config struct {
	Current           string            `json:"current"`
	CurrentNode       string            `json:"current_node"`
	Secret            string            `json:"secret"`
	OverrideDNS       bool              `json:"override_dns"`
	EnhancedMode      string            `json:"enhanced_mode"`
	ControllerPort    int               `json:"controller_port"`
	AllowRemoteAccess bool              `json:"allow_remote_access"`
	Dashboard         string            `json:"dashboard"`
	SkipSteamDownload bool              `json:"skip_steam_download"`
	Autostart         bool              `json:"autostart"`
	Subscriptions     map[string]string `json:"subscriptions"`
}

func Defaults() Config {
	return Config{
		EnhancedMode:   "fake-ip",
		ControllerPort: 9090,
		OverrideDNS:    true,
		Subscriptions:  map[string]string{},
	}
}

// RegisterSub добавляет подписку в config.json round-trip-безопасно: грузим
// весь файл как map (сохраняя чужие ключи плагина — log_level, timeout и пр.),
// дописываем подписку и, если current пуст, делаем её текущей.
func RegisterSub(name, url string) error {
	raw := map[string]any{}
	if data, err := os.ReadFile(paths.ConfigJSON()); err == nil {
		_ = json.Unmarshal(data, &raw)
	}
	subs, _ := raw["subscriptions"].(map[string]any)
	if subs == nil {
		subs = map[string]any{}
	}
	subs[name] = url
	raw["subscriptions"] = subs
	if cur, _ := raw["current"].(string); cur == "" {
		raw["current"] = name
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	// 0644 ОБЯЗАТЕЛЬНО: плагин пишет от root, а дек-юзер (TUI/ctl, юнит
	// ExecStartPre=regen) должен МОЧЬ ПРОЧИТАТЬ config.json. Запись дек-юзера
	// идёт через rename в его каталоге — работает поверх любого владельца.
	tmp := paths.ConfigJSON() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, paths.ConfigJSON())
}

// Load читает config.json из канонического каталога. Отсутствие файла — не
// ошибка (вернутся дефолты).
func Load() (Config, error) {
	c := Defaults()
	data, err := os.ReadFile(paths.ConfigJSON())
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if c.Subscriptions == nil {
		c.Subscriptions = map[string]string{}
	}
	return c, nil
}
