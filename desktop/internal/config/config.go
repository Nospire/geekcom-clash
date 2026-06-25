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
