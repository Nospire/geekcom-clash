// Package subscription — порт py_modules/subscription.py (add/download часть).
// Различает share-ссылку/base64 и http-URL, валидирует через mihomo -t,
// сохраняет yaml в subscriptions/.
package subscription

import (
	"crypto/rand"
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
	Name string
	URL  string // "local://<name>" для share-ссылок, исходный URL для http
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
	return Result{Name: name, URL: "local://" + name}, nil
}

func addFromURL(url string, existing map[string]string) (Result, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Result{}, fmt.Errorf("bad url: %w", err)
	}
	req.Header.Set("User-Agent", userAgent())
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
	if sharelink.LooksLikeSharelink(string(body)) {
		if proxies, _ := sharelink.Parse(string(body)); len(proxies) > 0 {
			if y, e := sharelink.BuildYAML(proxies); e == nil {
				body = y
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
	return Result{Name: filename, URL: url}, nil
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
