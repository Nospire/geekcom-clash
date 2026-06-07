package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const importerPort = 50581

// externalDir — каталог статики импортёра (тот же, что у плагина).
func externalDir() string {
	if d := os.Getenv("DECKY_PLUGIN_DIR"); d != "" {
		return filepath.Join(d, "external")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "homebrew", "plugins", "GeekcomClash", "external")
}

// lanIP — реальный LAN-IPv4 (192.168/10/172.16) для ссылки на телефон.
// НЕ udp-dial к 8.8.8.8: при включённом VPN маршрут уходит в tun Meta и вернётся
// его адрес 198.18.x. Перебираем интерфейсы, пропуская tun/VPN/виртуальные.
func lanIP() string {
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
			strings.Contains(n, "utun") || strings.Contains(n, "wg") ||
			strings.Contains(n, "docker") || strings.Contains(n, "veth") {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			if ip4[0] == 192 && ip4[1] == 168 ||
				ip4[0] == 10 ||
				ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
				return ip4.String()
			}
		}
	}
	return "127.0.0.1"
}

// startImporter поднимает мини веб-сервер импортёра подписок.
func startImporter() (*http.Server, string, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/download_sub", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		link := strings.TrimSpace(r.URL.Query().Get("link"))
		if link == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"empty link"}`))
			return
		}
		out, err := runCtl("add-sub", link)
		if err == nil && (strings.Contains(out, `"ok": true`) || strings.Contains(out, `"ok":true`)) {
			w.WriteHeader(200)
			w.Write([]byte(`{"ok":true}`))
			return
		}
		msg := strings.TrimSpace(out)
		if err != nil {
			msg = err.Error()
		}
		w.WriteHeader(500)
		w.Write([]byte(fmt.Sprintf(`{"error":%q}`, msg)))
	})
	// Импорт файла — пока только через плагин (требует import-file в ctl).
	mux.HandleFunc("/upload_sub", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(501)
		w.Write([]byte(`{"error":"Импорт файла пока только через плагин. Используйте ссылку."}`))
	})
	mux.Handle("/", http.FileServer(http.Dir(externalDir())))

	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", importerPort))
	if err != nil {
		return nil, "", err
	}
	go srv.Serve(ln)
	return srv, fmt.Sprintf("http://%s:%d", lanIP(), importerPort), nil
}
