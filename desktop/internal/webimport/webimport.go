// Package webimport — ЕДИНЫЙ модуль веб-импорта подписки с телефона, общий для
// GUI (in-process) и плагина (через CLI `geekcom-clash webimport`). Отдаёт ту
// же статику external/, что и релизная версия (index.html/app.js/styles.css),
// а ссылки принимает на /download_sub и добавляет через движок.
package webimport

import (
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"

	"geekcom-clash/internal/config"
	"geekcom-clash/internal/subscription"
)

//go:embed external
var externalFS embed.FS

// LanIP — первый приватный IPv4 активного интерфейса (пропускаем tun/VPN/вирт.,
// иначе при включённом VPN вернётся адрес tun Meta 198.18.x).
func LanIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		n := strings.ToLower(ifc.Name)
		if strings.Contains(n, "meta") || strings.Contains(n, "tun") ||
			strings.Contains(n, "wg") || strings.Contains(n, "docker") ||
			strings.Contains(n, "veth") {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil {
				continue
			}
			if ip[0] == 192 && ip[1] == 168 || ip[0] == 10 ||
				ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}

// Start поднимает веб-сервер импорта (релизная страница external/ + эндпоинты).
// onAdded зовётся после успешного добавления. Возвращает URL для телефона и stop().
func Start(onAdded func(name string)) (url string, stop func(), err error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	// Контракт как в релизе: GET /download_sub?link=… → 200 {"ok":true} | {"error":…}
	mux.HandleFunc("/download_sub", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		link := strings.TrimSpace(r.URL.Query().Get("link"))
		if link == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"empty link"}`)
			return
		}
		c, _ := config.Load()
		res, e := subscription.Add(link, c.Subscriptions)
		if e != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error":%q}`, e.Error())
			return
		}
		config.RegisterSub(res.Name, res.URL)
		if onAdded != nil {
			onAdded(res.Name)
		}
		fmt.Fprint(w, `{"ok":true}`)
	})
	mux.HandleFunc("/upload_sub", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		fmt.Fprint(w, `{"error":"Импорт файла пока только через ссылку."}`)
	})
	sub, _ := fs.Sub(externalFS, "external")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	return fmt.Sprintf("http://%s:%d", LanIP(), port), func() { srv.Close() }, nil
}
