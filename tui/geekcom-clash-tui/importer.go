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

// lanIP — локальный IPv4 для ссылки (чтобы открыть с телефона).
func lanIP() string {
	c, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
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
