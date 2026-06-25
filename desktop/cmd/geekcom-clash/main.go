// geekcom-clash — единый Go-движок (nightly-каркас). Будущий моно-бинарь:
// CLI-движок (regen/start/stop/подписки/ноды) + десктоп-GUI (Fyne) одним
// бинарём. Плагин Decky шеллит сюда же → один источник истины, согласование.
//
// Сейчас реализовано: regen (генерация running_config из общего config.json)
// + сервисные команды-заглушки. Это де-риск самого сложного куска — порт
// generate_config на Go.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"geekcom-clash/internal/config"
	"geekcom-clash/internal/engine"
	"geekcom-clash/internal/paths"
	"geekcom-clash/internal/service"
	"geekcom-clash/internal/subscription"
	"geekcom-clash/internal/webimport"
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
	case "add-sub":
		err = cmdAddSub(os.Args[2:])
	case "start":
		err = service.Start()
	case "stop":
		err = service.Stop()
	case "restart":
		err = service.Restart()
	case "install-unit":
		err = service.InstallUnit()
	case "status":
		if service.IsActive() {
			fmt.Println("active")
		} else {
			fmt.Println("inactive")
		}
	case "webimport":
		// ЕДИНЫЙ модуль импорта (тот же, что в GUI). Плагин шеллит сюда:
		// печатаем URL для телефона в stdout и блокируемся до kill.
		u, _, e := webimport.Start(func(name string) { fmt.Println("added:", name) })
		if e != nil {
			err = e
		} else {
			fmt.Println(u)
			select {}
		}
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

// cmdAddSub: добавить подписку (share-ссылка/base64/http-URL). Печатает JSON
// {"ok":bool,"result":...} как Python-ctl. Ошибка добавления — это ok:false,
// exit 0 (контракт по JSON); инфраструктурная ошибка — exit 1.
func cmdAddSub(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: geekcom-clash add-sub <url>")
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	res, addErr := subscription.Add(args[0], c.Subscriptions)
	if addErr != nil {
		printJSON(map[string]any{"ok": false, "result": addErr.Error()})
		return nil
	}
	if err := config.RegisterSub(res.Name, res.URL); err != nil {
		return err
	}
	printJSON(map[string]any{"ok": true, "result": []string{res.Name, res.URL}})
	return nil
}

func printJSON(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func cmdPaths() {
	fmt.Println("data:    ", paths.DataDir())
	fmt.Println("config:  ", paths.ConfigJSON())
	fmt.Println("subs:    ", paths.SubscriptionsDir())
	fmt.Println("running: ", paths.RunningConfig())
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: geekcom-clash <regen|add-sub|start|stop|restart|status|install-unit|webimport|paths|version>")
}
