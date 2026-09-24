// Package sharelink — порт py_modules/sharelink.py на Go.
// Парсит share-ссылки (vless:// vmess:// ss:// trojan:// hysteria2:// hy2://)
// и base64-подписки v2ray-формата в список Clash/Mihomo-нод и собирает
// минимальный валидный конфиг.
package sharelink

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var schemes = []string{"vless://", "vmess://", "ss://", "trojan://", "hysteria2://", "hy2://"}

// Proxy — clash-нода (произвольные поля; порядок ключей для mihomo неважен).
type Proxy = map[string]any

// ───────────── helpers ─────────────

func b64decode(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	pad := s
	if m := len(s) % 4; m != 0 {
		pad = s + strings.Repeat("=", 4-m)
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
		if b, err := enc.DecodeString(pad); err == nil {
			return string(b), true
		}
	}
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(strings.TrimRight(s, "=")); err == nil {
			return string(b), true
		}
	}
	return "", false
}

func nameFromFragment(u *url.URL, fallback string) string {
	if n := strings.TrimSpace(u.Fragment); n != "" {
		return n
	}
	return fallback
}

func qs(u *url.URL) map[string]string {
	out := map[string]string{}
	for k, vs := range u.Query() {
		if len(vs) > 0 {
			out[k] = vs[len(vs)-1]
		}
	}
	return out
}

func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func splitCSV(s string) []any {
	var out []any
	for _, p := range strings.Split(s, ",") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hasSchemePrefix(s string) bool {
	low := strings.ToLower(s)
	for _, sc := range schemes {
		if strings.HasPrefix(low, sc) {
			return true
		}
	}
	return false
}

func splitLines(text string) []string {
	text = strings.TrimSpace(text)
	if !hasSchemePrefix(text) {
		if dec, ok := b64decode(text); ok {
			low := strings.ToLower(dec)
			for _, sc := range schemes {
				if strings.Contains(low, sc) {
					text = dec
					break
				}
			}
		}
	}
	var lines []string
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r", "\n"), "\n") {
		ln := strings.TrimSpace(raw)
		if ln != "" && hasSchemePrefix(ln) {
			lines = append(lines, ln)
		}
	}
	return lines
}

// LooksLikeSharelink — похоже ли на share-ссылку / base64-список.
func LooksLikeSharelink(text string) bool {
	return len(splitLines(text)) > 0
}

// ───────────── транспорт / tls ─────────────

func netOpts(p Proxy, params map[string]string, network string) {
	host, path := params["host"], params["path"]
	switch network {
	case "ws", "httpupgrade":
		ws := Proxy{}
		if path != "" {
			ws["path"] = path
		}
		if host != "" {
			ws["headers"] = Proxy{"Host": host}
		}
		if len(ws) > 0 {
			p["ws-opts"] = ws
		}
	case "grpc":
		svc := params["serviceName"]
		if svc == "" {
			svc = params["servicename"]
		}
		if svc == "" {
			svc = path
		}
		if svc != "" {
			p["grpc-opts"] = Proxy{"grpc-service-name": svc}
		}
	case "http", "h2":
		h2 := Proxy{}
		if path != "" {
			h2["path"] = path
		}
		if host != "" {
			h2["host"] = splitCSV(host)
		}
		if len(h2) > 0 {
			p["h2-opts"] = h2
		}
	}
}

func tlsOpts(p Proxy, params map[string]string, defaultSNI string) {
	security := strings.ToLower(params["security"])
	sni := params["sni"]
	if sni == "" {
		sni = params["peer"]
	}
	if sni == "" {
		sni = defaultSNI
	}
	if security == "tls" || security == "reality" || security == "xtls" {
		p["tls"] = true
	}
	if sni != "" {
		p["servername"] = sni
	}
	if fp := params["fp"]; fp != "" {
		p["client-fingerprint"] = fp
	}
	if alpn := params["alpn"]; alpn != "" {
		p["alpn"] = splitCSV(alpn)
	}
	if truthy(params["allowInsecure"]) || truthy(params["insecure"]) {
		p["skip-cert-verify"] = true
	}
	if security == "reality" {
		ro := Proxy{}
		if v := params["pbk"]; v != "" {
			ro["public-key"] = v
		}
		if v := params["sid"]; v != "" {
			ro["short-id"] = v
		}
		if len(ro) > 0 {
			p["reality-opts"] = ro
		}
	}
}

// ───────────── парсеры ─────────────

func parseVless(raw string, idx int) Proxy {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Port() == "" {
		return nil
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil
	}
	p := qs(u)
	network := strings.ToLower(p["type"])
	if network == "" {
		network = "tcp"
	}
	net := network
	if net == "httpupgrade" {
		net = "ws"
	}
	uuid := ""
	if u.User != nil {
		uuid = u.User.Username()
	}
	proxy := Proxy{
		"name": nameFromFragment(u, "vless-"+strconv.Itoa(idx)),
		"type": "vless", "server": u.Hostname(), "port": port,
		"uuid": uuid, "network": net, "udp": true,
	}
	if flow := p["flow"]; flow != "" {
		proxy["flow"] = flow
	}
	tlsOpts(proxy, p, p["host"])
	netOpts(proxy, p, network)
	return proxy
}

func parseVmess(raw string, idx int) Proxy {
	dec, ok := b64decode(strings.TrimPrefix(raw, "vmess://"))
	if !ok {
		return nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(dec), &cfg); err != nil {
		return nil
	}
	server := asStr(cfg["add"])
	port := asInt(cfg["port"])
	if server == "" || port == 0 {
		return nil
	}
	network := strings.ToLower(asStr(cfg["net"]))
	if network == "" {
		network = "tcp"
	}
	net := network
	if net == "httpupgrade" {
		net = "ws"
	}
	name := strings.TrimSpace(asStr(cfg["ps"]))
	if name == "" {
		name = "vmess-" + strconv.Itoa(idx)
	}
	proxy := Proxy{
		"name": name, "type": "vmess", "server": server, "port": port,
		"uuid": asStr(cfg["id"]), "alterId": asInt(cfg["aid"]),
		"cipher": orStr(asStr(cfg["scy"]), "auto"), "network": net, "udp": true,
	}
	if t := strings.ToLower(asStr(cfg["tls"])); t == "tls" || t == "reality" {
		proxy["tls"] = true
		sni := orStr(asStr(cfg["sni"]), asStr(cfg["host"]))
		if sni != "" {
			proxy["servername"] = sni
		}
		if a := asStr(cfg["alpn"]); a != "" {
			proxy["alpn"] = splitCSV(a)
		}
		if fp := asStr(cfg["fp"]); fp != "" {
			proxy["client-fingerprint"] = fp
		}
	}
	netOpts(proxy, map[string]string{"host": asStr(cfg["host"]), "path": asStr(cfg["path"]), "serviceName": asStr(cfg["path"])}, network)
	return proxy
}

func parseSS(raw string, idx int) Proxy {
	u0, _ := url.Parse(raw)
	name := "ss-" + strconv.Itoa(idx)
	if u0 != nil {
		name = nameFromFragment(u0, name)
	}
	body := strings.TrimPrefix(raw, "ss://")
	if i := strings.Index(body, "#"); i >= 0 {
		body = body[:i]
	}
	query := ""
	if i := strings.Index(body, "?"); i >= 0 {
		query = body[i+1:]
		body = body[:i]
	}
	var method, password, host string
	var port int
	if i := strings.LastIndex(body, "@"); i >= 0 {
		userinfo, hostport := body[:i], body[i+1:]
		creds, ok := b64decode(userinfo)
		if !ok {
			creds, _ = url.QueryUnescape(userinfo)
		}
		if j := strings.Index(creds, ":"); j >= 0 {
			method, password = creds[:j], creds[j+1:]
		}
		if j := strings.LastIndex(hostport, ":"); j >= 0 {
			host = hostport[:j]
			port, _ = strconv.Atoi(hostport[j+1:])
		}
	} else if dec, ok := b64decode(body); ok && strings.Contains(dec, "@") && strings.Contains(dec, ":") {
		i := strings.LastIndex(dec, "@")
		creds, hostport := dec[:i], dec[i+1:]
		if j := strings.Index(creds, ":"); j >= 0 {
			method, password = creds[:j], creds[j+1:]
		}
		if j := strings.LastIndex(hostport, ":"); j >= 0 {
			host = hostport[:j]
			port, _ = strconv.Atoi(hostport[j+1:])
		}
	}
	if method == "" || host == "" || port == 0 {
		return nil
	}
	proxy := Proxy{
		"name": name, "type": "ss", "server": host, "port": port,
		"cipher": method, "password": password, "udp": true,
	}
	if query != "" {
		qp, _ := url.ParseQuery(query)
		if plug := qp.Get("plugin"); plug != "" {
			parts := strings.Split(plug, ";")
			pname := parts[0]
			opts := map[string]string{}
			for _, o := range parts[1:] {
				if j := strings.Index(o, "="); j >= 0 {
					opts[o[:j]] = o[j+1:]
				}
			}
			switch pname {
			case "obfs-local", "simple-obfs":
				proxy["plugin"] = "obfs"
				proxy["plugin-opts"] = Proxy{"mode": orStr(opts["obfs"], "http"), "host": opts["obfs-host"]}
			case "v2ray-plugin":
				_, tls := opts["tls"]
				proxy["plugin"] = "v2ray-plugin"
				proxy["plugin-opts"] = Proxy{"mode": orStr(opts["mode"], "websocket"), "host": opts["host"], "path": orStr(opts["path"], "/"), "tls": tls}
			}
		}
	}
	return proxy
}

func parseTrojan(raw string, idx int) Proxy {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Port() == "" {
		return nil
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil
	}
	p := qs(u)
	network := strings.ToLower(p["type"])
	if network == "" {
		network = "tcp"
	}
	pass := ""
	if u.User != nil {
		pass = u.User.Username()
	}
	proxy := Proxy{
		"name": nameFromFragment(u, "trojan-"+strconv.Itoa(idx)),
		"type": "trojan", "server": u.Hostname(), "port": port,
		"password": pass, "udp": true,
	}
	sni := orStr(p["sni"], p["peer"])
	if sni != "" {
		proxy["sni"] = sni
	}
	if a := p["alpn"]; a != "" {
		proxy["alpn"] = splitCSV(a)
	}
	if fp := p["fp"]; fp != "" {
		proxy["client-fingerprint"] = fp
	}
	if truthy(p["allowInsecure"]) || truthy(p["insecure"]) {
		proxy["skip-cert-verify"] = true
	}
	switch network {
	case "ws", "grpc", "http", "h2", "httpupgrade":
		net := network
		if net == "httpupgrade" {
			net = "ws"
		}
		proxy["network"] = net
		netOpts(proxy, p, network)
	}
	return proxy
}

func parseHysteria2(raw string, idx int) Proxy {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Port() == "" {
		return nil
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil
	}
	p := qs(u)
	pass := ""
	if u.User != nil {
		pass = u.User.Username()
		if pw, has := u.User.Password(); pass == "" && has {
			pass = pw
		}
	}
	proxy := Proxy{
		"name": nameFromFragment(u, "hy2-"+strconv.Itoa(idx)),
		"type": "hysteria2", "server": u.Hostname(), "port": port, "password": pass,
	}
	sni := orStr(p["sni"], p["peer"])
	if sni != "" {
		proxy["sni"] = sni
	}
	if truthy(p["insecure"]) || truthy(p["allowInsecure"]) {
		proxy["skip-cert-verify"] = true
	}
	if obfs := p["obfs"]; obfs != "" {
		proxy["obfs"] = obfs
		if op := p["obfs-password"]; op != "" {
			proxy["obfs-password"] = op
		}
	}
	return proxy
}

func parseOne(line string, idx int) Proxy {
	low := strings.ToLower(line)
	switch {
	case strings.HasPrefix(low, "vless://"):
		return parseVless(line, idx)
	case strings.HasPrefix(low, "vmess://"):
		return parseVmess(line, idx)
	case strings.HasPrefix(low, "ss://"):
		return parseSS(line, idx)
	case strings.HasPrefix(low, "trojan://"):
		return parseTrojan(line, idx)
	case strings.HasPrefix(low, "hysteria2://"), strings.HasPrefix(low, "hy2://"):
		return parseHysteria2(line, idx)
	}
	return nil
}

func dedupNames(proxies []Proxy) {
	seen := map[string]int{}
	for _, pr := range proxies {
		base := asStr(pr["name"])
		if seen[base] > 0 {
			seen[base]++
			pr["name"] = base + " #" + strconv.Itoa(seen[base])
		} else {
			seen[base] = 1
		}
	}
}

// Parse — разобрать текст в список нод + предложенное имя.
func Parse(text string) ([]Proxy, string) {
	lines := splitLines(text)
	var proxies []Proxy
	for i, ln := range lines {
		if pr := parseOne(ln, i+1); pr != nil {
			proxies = append(proxies, pr)
		}
	}
	dedupNames(proxies)
	name := "sharelink"
	if len(proxies) == 1 {
		name = asStr(proxies[0]["name"])
	} else if len(proxies) > 1 {
		name = asStr(proxies[0]["name"]) + " +" + strconv.Itoa(len(proxies)-1)
	}
	return proxies, name
}

// BuildYAML — собрать минимальный Clash-конфиг из списка нод.
func BuildYAML(proxies []Proxy) ([]byte, error) {
	names := make([]any, 0, len(proxies))
	for _, p := range proxies {
		names = append(names, asStr(p["name"]))
	}
	selectProxies := append(append([]any{}, names...), "DIRECT")
	autoProxies := names
	if len(autoProxies) == 0 {
		autoProxies = []any{"DIRECT"}
	}
	cfg := Proxy{
		"mixed-port": 7890,
		"mode":       "rule",
		"proxies":    toAnySlice(proxies),
		"proxy-groups": []any{
			Proxy{"name": "PROXY", "type": "select", "proxies": selectProxies},
			Proxy{"name": "AUTO", "type": "url-test", "url": "http://www.gstatic.com/generate_204", "interval": 300, "lazy": true, "proxies": autoProxies},
		},
		"rules": []any{"MATCH,PROXY"},
	}
	return yaml.Marshal(cfg)
}

// ───────────── мелкие приведения типов ─────────────

func asStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case nil:
		return ""
	default:
		return ""
	}
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	}
	return 0
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func toAnySlice(ps []Proxy) []any {
	out := make([]any, len(ps))
	for i, p := range ps {
		out[i] = p
	}
	return out
}
