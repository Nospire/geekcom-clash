// Package subscription — порт py_modules/subscription.py (add/download часть).
// Различает share-ссылку/base64 и http-URL, валидирует через mihomo -t,
// сохраняет yaml в subscriptions/.
package subscription

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"geekcom-clash/internal/paths"
	"geekcom-clash/internal/sharelink"
)

// Result — итог добавления подписки.
type Result struct {
	Name    string
	URL     string // "local://<name>" для share-ссылок, исходный URL для http
	NoRules bool   // у подписки нет правил маршрутизации (голый список нод) → весь трафик через VPN
}

// Add добавляет подписку (share-ссылка/base64 или http-URL).
func Add(input string, existing map[string]string) (Result, error) {
	if err := os.MkdirAll(paths.SubscriptionsDir(), 0o755); err != nil {
		return Result{}, err
	}
	if sharelink.LooksLikeSharelink(input) {
		return addFromSharelink(input, existing)
	}
	return addFromURL(input, existing)
}

func addFromSharelink(input string, existing map[string]string) (Result, error) {
	proxies, suggested := sharelink.Parse(input)
	if len(proxies) == 0 {
		return Result{}, fmt.Errorf("sharelink: не распознано ни одной ноды")
	}
	name := dedupName(existing, sanitizeFilename(suggested))
	if name == "" {
		return Result{}, fmt.Errorf("нет свободного имени")
	}
	yamlData, err := sharelink.BuildYAML(proxies)
	if err != nil {
		return Result{}, err
	}
	p := paths.SubPath(name)
	if err := os.WriteFile(p, yamlData, 0o644); err != nil {
		return Result{}, fmt.Errorf("io: %w", err)
	}
	if err := validate(p); err != nil {
		os.Remove(p)
		return Result{}, fmt.Errorf("конфиг невалиден: %w", err)
	}
	// share-ссылки — это голый список нод без правил → весь трафик через VPN.
	return Result{Name: name, URL: "local://" + name, NoRules: true}, nil
}

// hasRoutingRules — есть ли в конфиге настоящие правила маршрутизации (не только
// финальный MATCH). Голый список нод (share/base64) даёт лишь MATCH,GEEKCOM-VPN.
func hasRoutingRules(body []byte) bool {
	for _, kw := range []string{"RULE-SET,", "GEOIP,", "GEOSITE,", "DOMAIN,", "DOMAIN-SUFFIX,", "DOMAIN-KEYWORD,", "IP-CIDR,", "IP-CIDR6,", "PROCESS-NAME,", "SRC-IP-CIDR,"} {
		if bytes.Contains(body, []byte(kw)) {
			return true
		}
	}
	return false
}

func addFromURL(url string, existing map[string]string) (Result, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Result{}, fmt.Errorf("bad url: %w", err)
	}
	setSubHeaders(req)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Result{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("read: %w", err)
	}

	// Некоторые провайдеры отдают НЕ clash-YAML, а base64-подписку v2ray (список
	// vless://,vmess://,ss://,trojan://,hysteria2:// ). mihomo такой формат не
	// понимает → "конфиг невалиден: … test failed". Детектим и конвертируем в
	// clash-конфиг (декод base64 + разбор ссылок + BuildYAML).
	converted := false
	if sharelink.LooksLikeSharelink(string(body)) {
		if proxies, _ := sharelink.Parse(string(body)); len(proxies) > 0 {
			if y, e := sharelink.BuildYAML(proxies); e == nil {
				body = y
				converted = true
			}
		}
	}

	filename := filenameFromResp(resp, url)
	filename = strings.TrimSuffix(filename, ".yml")
	filename = strings.TrimSuffix(filename, ".yaml")
	if filename == "" {
		filename = randThing()
	}
	filename = dedupName(existing, sanitizeFilename(filename))
	if filename == "" {
		return Result{}, fmt.Errorf("нет свободного имени")
	}

	p := paths.SubPath(filename)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		return Result{}, fmt.Errorf("io: %w", err)
	}
	if err := validate(p); err != nil {
		os.Remove(p)
		return Result{}, fmt.Errorf("конфиг невалиден: %w", err)
	}
	// Нет правил маршрутизации, если сконвертировали из голого списка нод ИЛИ в
	// clash-конфиге провайдера нет ни одного правила (только финальный MATCH).
	noRules := converted || !hasRoutingRules(body)
	return Result{Name: filename, URL: url, NoRules: noRules}, nil
}

// validate — mihomo -t. Бинарь и resource-dir берём из env (плагин их знает).
// Если бинаря нет (env не задан ИЛИ файл отсутствует) — валидацию ПРОПУСКАЕМ:
// подписка сама по себе валидна, а отсутствие ядра — отдельная проблема (лечится
// переустановкой/самолечением), блокировать импорт из-за неё нельзя. Раньше при
// отсутствии mihomo exec падал с пустым выводом → пользователь видел загадочное
// «конфиг невалиден:» без текста.
func validate(configPath string) error {
	bin := os.Getenv("GEEKCOM_CLASH_MIHOMO")
	if bin == "" {
		return nil
	}
	if _, statErr := os.Stat(bin); statErr != nil {
		return nil // mihomo не установлен — пропускаем (импорт не должен падать)
	}
	d := os.Getenv("GEEKCOM_CLASH_RESOURCE_DIR")
	if d == "" {
		d = filepath.Dir(configPath)
	}
	out, err := exec.Command(bin, "-t", "-f", configPath, "-d", d).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(lastLine(string(out)))
		if msg == "" {
			msg = strings.TrimSpace(string(out))
		}
		if msg == "" {
			msg = err.Error() // не оставляем сообщение пустым (напр. exec-ошибка)
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func filenameFromResp(resp *http.Response, url string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if fn := params["filename"]; fn != "" {
				return fn
			}
		}
	}
	if i := strings.IndexAny(url, "?#"); i >= 0 {
		url = url[:i]
	}
	return path.Base(url)
}

// deviceHWID — стабильный идентификатор устройства для HWID-лимита Remnawave.
// sha256("geekcom-clash:"+machine-id): одинаков для Go и Python (тот же machine-id),
// стабилен между запусками. Fallback — персистентный uuid в data-каталоге.
func deviceHWID() string {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(p); err == nil {
			if id := strings.TrimSpace(string(b)); id != "" {
				sum := sha256.Sum256([]byte("geekcom-clash:" + id))
				return hex.EncodeToString(sum[:])
			}
		}
	}
	hp := filepath.Join(paths.DataDir(), ".hwid")
	if b, err := os.ReadFile(hp); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id
		}
	}
	buf := make([]byte, 16)
	rand.Read(buf)
	id := hex.EncodeToString(buf)
	_ = os.MkdirAll(paths.DataDir(), 0o755)
	_ = os.WriteFile(hp, []byte(id), 0o644)
	return id
}

// setSubHeaders — заголовки запроса подписки: clash-UA + HWID-набор Remnawave.
// Заголовки безвредны, если у провайдера HWID-лимит выключен. Провайдер считает
// устройства по x-hwid.
func setSubHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("x-hwid", deviceHWID())
	req.Header.Set("x-device-os", "SteamOS")
	req.Header.Set("x-ver-os", "3.0")
	req.Header.Set("x-device-model", "Steam Deck")
}

func userAgent() string {
	return "mihomo clash.meta clash-verge/2.5.0 mihomo.party/v1.9.5 geekcom-clash"
}

var unsafeFilename = regexp.MustCompile(`[^\w.\-+ а-яА-ЯёЁ]`)

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = unsafeFilename.ReplaceAllString(name, "")
	name = strings.TrimSpace(name)
	if name == "" {
		name = randThing()
	}
	return name
}

func dedupName(existing map[string]string, name string) string {
	if _, ok := existing[name]; !ok {
		return name
	}
	for i := 0; i < 100; i++ {
		cand := fmt.Sprintf("%s_%d", name, i)
		if _, ok := existing[cand]; !ok {
			return cand
		}
	}
	return ""
}

func randThing() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "sub"
	}
	return "sub-" + hex.EncodeToString(b)
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}
	return s
}
