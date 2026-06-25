// geekcom-clash — единый Go-движок (nightly-каркас). Будущий моно-бинарь:
// CLI-движок (regen/start/stop/подписки/ноды) + десктоп-GUI (Fyne) одним
// бинарём. Плагин Decky шеллит сюда же → один источник истины, согласование.
//
// Сейчас реализовано: regen (генерация running_config из общего config.json)
// + сервисные команды-заглушки. Это де-риск самого сложного куска — порт
// generate_config на Go.
package main

import (
	"flag"
	"fmt"
	"os"

	"geekcom-clash/internal/config"
	"geekcom-clash/internal/engine"
	"geekcom-clash/internal/paths"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "regen":
		err = cmdRegen(os.Args[2:])
	case "paths":
		cmdPaths()
	case "version", "-v", "--version":
		fmt.Println("geekcom-clash engine (nightly)")
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdRegen(args []string) error {
	fs := flag.NewFlagSet("regen", flag.ContinueOnError)
	out := fs.String("out", "", "путь для running_config (по умолчанию <data>/running_config.yaml)")
	dashDir := fs.String("dashboard-dir", "", "каталог дашборда для external-ui")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	if c.Current == "" {
		return fmt.Errorf("подписка не выбрана (current пуст)")
	}
	newPath := paths.RunningConfig()
	if *out != "" {
		newPath = *out
	}
	if err := engine.GenerateConfig(engine.Opts{
		OriPath:           paths.SubPath(c.Current),
		NewPath:           newPath,
		Secret:            c.Secret,
		OverrideDNS:       c.OverrideDNS,
		EnhancedMode:      c.EnhancedMode,
		ControllerPort:    c.ControllerPort,
		AllowRemoteAccess: c.AllowRemoteAccess,
		Dashboard:         c.Dashboard,
		DashboardDir:      *dashDir,
		SkipSteamDownload: c.SkipSteamDownload,
	}); err != nil {
		return err
	}
	fmt.Println(newPath)
	return nil
}

func cmdPaths() {
	fmt.Println("data:    ", paths.DataDir())
	fmt.Println("config:  ", paths.ConfigJSON())
	fmt.Println("subs:    ", paths.SubscriptionsDir())
	fmt.Println("running: ", paths.RunningConfig())
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: geekcom-clash <regen|paths|version>")
}
