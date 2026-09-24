// Package paths задаёт КАНОНИЧЕСКОЕ расположение данных, общее для всех морд
// (плагин Decky / десктоп-GUI / CLI). Независимо от Decky Loader.
package paths

import (
	"os"
	"path/filepath"
)

// DataDir — канонический каталог данных.
// Приоритет: $GEEKCOM_CLASH_DIR → $XDG_CONFIG_HOME/geekcom-clash → ~/.config/geekcom-clash.
// Переменная окружения позволяет плагину/тестам указать общий путь (напр. путь Decky).
func DataDir() string {
	if d := os.Getenv("GEEKCOM_CLASH_DIR"); d != "" {
		return d
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "geekcom-clash")
}

func ConfigJSON() string         { return filepath.Join(DataDir(), "config.json") }
func SubscriptionsDir() string   { return filepath.Join(DataDir(), "subscriptions") }
func SubPath(name string) string { return filepath.Join(SubscriptionsDir(), name+".yaml") }
func RunningConfig() string      { return filepath.Join(DataDir(), "running_config.yaml") }
