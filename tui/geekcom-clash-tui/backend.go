package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// --- ctl: общий Python-хелпер (единый источник правды с плагином) ----------

func ctlPath() string {
	// рядом с бинарём TUI или в ~/gcc-tui
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "geekcom-clash-ctl")
		if _, e := os.Stat(p); e == nil {
			return p
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "gcc-tui", "geekcom-clash-ctl")
}

func runCtl(args ...string) (string, error) {
	cmd := exec.Command(ctlPath(), args...)
	cmd.Env = bgEnv()
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%v: %s", err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// Info — то, что отдаёт `ctl info`.
type Info struct {
	Secret         string `json:"secret"`
	ControllerPort int    `json:"controller_port"`
	Current        string `json:"current"`
	Dashboard      string `json:"dashboard"`
	Active         bool   `json:"active"`
	ControllerUp   bool   `json:"controller_up"`
	CoreBin        string `json:"core_bin"`
	RunningConfig  string `json:"running_config"`
}

func ctlInfo() (Info, error) {
	var i Info
	out, err := runCtl("info")
	if err != nil {
		return i, err
	}
	return i, json.Unmarshal([]byte(out), &i)
}

// SubList — то, что отдаёт `ctl list-subs`.
type SubList struct {
	Current       string            `json:"current"`
	Subscriptions map[string]string `json:"subscriptions"`
}

func ctlListSubs() (SubList, error) {
	var s SubList
	out, err := runCtl("list-subs")
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal([]byte(out), &s)
}

func ctlListDashboards() []string {
	out, err := runCtl("list-dashboards")
	if err != nil {
		return nil
	}
	var d []string
	json.Unmarshal([]byte(out), &d)
	return d
}

// dashURL — полный URL дашборда с hostname/port/secret, чтобы он сразу
// подключился к контроллеру (без формы логина).
func dashURL(i Info, name string) string {
	port := i.ControllerPort
	if name == "" {
		return fmt.Sprintf("http://127.0.0.1:%d/ui/", port)
	}
	path := "#/setup"
	if name == "yacd" {
		path = ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d/ui/%s/%s?hostname=127.0.0.1&port=%d&secret=%s",
		port, name, path, port, i.Secret)
}

// --- mihomo REST API -------------------------------------------------------

type apiClient struct {
	base   string
	secret string
	http   *http.Client
}

func newAPI(i Info) *apiClient {
	return &apiClient{
		base:   fmt.Sprintf("http://127.0.0.1:%d", i.ControllerPort),
		secret: i.Secret,
		http:   &http.Client{Timeout: 6 * time.Second},
	}
}

func (a *apiClient) do(method, path string, body []byte) (*http.Response, error) {
	var r *http.Request
	var err error
	if body != nil {
		r, err = http.NewRequest(method, a.base+path, bytes.NewReader(body))
	} else {
		r, err = http.NewRequest(method, a.base+path, nil)
	}
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+a.secret)
	r.Header.Set("Content-Type", "application/json")
	return a.http.Do(r)
}

// Proxy — узел или группа из /proxies.
type Proxy struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	All     []string `json:"all"`
	History []struct {
		Delay int `json:"delay"`
	} `json:"history"`
}

func (p Proxy) delay() int {
	if len(p.History) == 0 {
		return -1
	}
	return p.History[len(p.History)-1].Delay
}

func (a *apiClient) proxies() (map[string]Proxy, error) {
	resp, err := a.do("GET", "/proxies", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var wrap struct {
		Proxies map[string]Proxy `json:"proxies"`
	}
	return wrap.Proxies, json.NewDecoder(resp.Body).Decode(&wrap)
}

func (a *apiClient) mode() string {
	resp, err := a.do("GET", "/configs", nil)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var c struct {
		Mode string `json:"mode"`
	}
	json.NewDecoder(resp.Body).Decode(&c)
	return c.Mode
}

// forceGroup — имя url-test группы из override.yaml (force-proxy-group.name).
const forceGroup = "GEEKCOM-VPN"

func (a *apiClient) setMode(m string) error {
	if _, err := a.do("PATCH", "/configs", []byte(fmt.Sprintf(`{"mode":%q}`, m))); err != nil {
		return err
	}
	// В режиме global трафик идёт через встроенный селектор GLOBAL, а он по
	// умолчанию (и из-за store-selected) может указывать на DIRECT → VPN молча
	// не работает. Направляем GLOBAL на нашу VPN-группу, чтобы «Global» реально
	// гнал весь трафик через VPN. Best-effort: не валим переключение, если не вышло.
	if m == "global" {
		_ = a.selectNode("GLOBAL", forceGroup)
	}
	return nil
}

func (a *apiClient) selectNode(group, name string) error {
	_, err := a.do("PUT", "/proxies/"+urlEscape(group), []byte(fmt.Sprintf(`{"name":%q}`, name)))
	return err
}

// Traffic — одна выборка скорости из стрима /traffic.
type Traffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

func (a *apiClient) traffic() (Traffic, error) {
	var t Traffic
	resp, err := a.do("GET", "/traffic", nil)
	if err != nil {
		return t, err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	if sc.Scan() { // одна строка-выборка достаточно
		json.Unmarshal(sc.Bytes(), &t)
	}
	return t, nil
}

// groupDelay — тест задержки всех нод группы: /group/{name}/delay.
func (a *apiClient) groupDelay(group string) (map[string]int, error) {
	path := "/group/" + urlEscape(group) + "/delay?url=http://www.gstatic.com/generate_204&timeout=3000"
	resp, err := a.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	m := map[string]int{}
	return m, json.NewDecoder(resp.Body).Decode(&m)
}

// primarySelector — главная пользовательская группа-селектор.
func primarySelector(px map[string]Proxy) string {
	prefer := []string{"→ Remnawave", "Remnawave", "PROXY", "Proxy", "Proxies", "节点选择", "选择节点"}
	for _, name := range prefer {
		if p, ok := px[name]; ok && p.Type == "Selector" {
			return name
		}
	}
	// иначе — первый Selector кроме GLOBAL, по имени для стабильности
	var sels []string
	for name, p := range px {
		if p.Type == "Selector" && name != "GLOBAL" && len(p.All) > 1 {
			sels = append(sels, name)
		}
	}
	sort.Strings(sels)
	if len(sels) > 0 {
		return sels[0]
	}
	return "GLOBAL"
}

// --- системные хелперы -----------------------------------------------------

func urlEscape(s string) string {
	return strings.NewReplacer(" ", "%20", "→", "%E2%86%92").Replace(s)
}

func clipboardPaste() string {
	for _, c := range [][]string{{"wl-paste", "-n"}, {"xclip", "-o", "-selection", "clipboard"}} {
		if _, err := exec.LookPath(c[0]); err == nil {
			out, err := exec.Command(c[0], c[1:]...).Output()
			if err == nil {
				return strings.TrimSpace(string(out))
			}
		}
	}
	return ""
}

func openURL(url string) {
	exec.Command("xdg-open", url).Start()
}

// latestRelease — тег последнего релиза с GitHub ("" при ошибке/оффлайне).
func latestRelease() string {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get("https://api.github.com/repos/Nospire/geekcom-clash/releases/latest")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var r struct {
		Tag string `json:"tag_name"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	return r.Tag
}

// openInstaller — запускает установщик в отдельном окне Konsole (там же ввод
// sudo-пароля и видно прогресс).
func openInstaller() {
	exec.Command("konsole", "-e", "bash", "-c",
		"curl -L https://gdt.geekcom.org/clash | bash; echo; read -p 'Готово. Enter для выхода...'").Start()
}

func openLogs() {
	// отдельное окно Konsole с живым логом юнита
	exec.Command("konsole", "-e", "bash", "-c",
		"journalctl --user -u geekcom-clash -f --no-pager; read").Start()
}

func bgEnv() []string {
	env := os.Environ()
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		env = append(env, fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", os.Getuid()))
	}
	return env
}

func sysctlUser(args ...string) (string, error) {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Env = bgEnv()
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func autostartEnabled() bool {
	out, _ := sysctlUser("is-enabled", "geekcom-clash.service")
	return out == "enabled"
}

func setAutostart(on bool) error {
	if on {
		_, err := sysctlUser("enable", "geekcom-clash.service")
		return err
	}
	_, err := sysctlUser("disable", "geekcom-clash.service")
	return err
}

// --- персист настроек TUI (тема) -------------------------------------------

type tuiConfig struct {
	Theme string `json:"theme"`
}

func tuiConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "geekcom-clash", "tui.json")
}

func loadTUIConfig() tuiConfig {
	c := tuiConfig{Theme: "dracula"}
	b, err := os.ReadFile(tuiConfigPath())
	if err == nil {
		json.Unmarshal(b, &c)
	}
	return c
}

func saveTUIConfig(c tuiConfig) {
	os.MkdirAll(filepath.Dir(tuiConfigPath()), 0o755)
	b, _ := json.Marshal(c)
	os.WriteFile(tuiConfigPath(), b, 0o644)
}
