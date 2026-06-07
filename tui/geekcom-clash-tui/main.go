package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const innerW = 64
const maxNodes = 7

// version — версия сборки (ставится через -ldflags -X main.version).
var version = "dev"

// Ссылки сообщества (зеркалят src/branding.ts).
const (
	linkBoosty  = "https://boosty.to/steamdecks"
	linkTGGames = "https://t.me/geekcom_deck_games"
	linkTGNews  = "https://t.me/geekcomdeck_news"
	linkTGChat  = "https://t.me/Geekcom_hub"
)

func normVer(s string) string { return strings.TrimPrefix(strings.TrimSpace(s), "v") }

// zone — кликабельная зона (абсолютные координаты терминала).
type zone struct {
	x0, y0, x1, y1 int
	id, data       string
}

type nodeItem struct {
	name     string
	delay    int
	selected bool
}

type model struct {
	th       Theme
	w, h     int
	screen   string // main | addsub | subpick
	info     Info
	api      *apiClient
	group    string
	nodes    []nodeItem
	nodeScr  int
	mode     string
	tr       Traffic
	subs     []string
	current  string
	dashes   []string
	autostrt bool
	latest   string
	busy     string
	status   string
	ti       textinput.Model
	zones    *[]zone
}

func (m model) updateAvailable() bool {
	l := normVer(m.latest)
	return l != "" && l != normVer(version)
}

// --- сообщения -------------------------------------------------------------

type tickMsg time.Time
type infoMsg struct {
	info Info
	err  error
}
type dataMsg struct {
	group   string
	nodes   []nodeItem
	mode    string
	tr      Traffic
	subs    []string
	current string
	dashes  []string
	auto    bool
}
type statusMsg string
type updateMsg string

func checkUpdateCmd() tea.Cmd {
	return func() tea.Msg { return updateMsg(latestRelease()) }
}

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// --- команды ---------------------------------------------------------------

func loadInfoCmd() tea.Cmd {
	return func() tea.Msg {
		i, err := ctlInfo()
		return infoMsg{i, err}
	}
}

func refreshCmd(i Info) tea.Cmd {
	return func() tea.Msg {
		d := dataMsg{}
		if sl, err := ctlListSubs(); err == nil {
			d.current = sl.Current
			for k := range sl.Subscriptions {
				d.subs = append(d.subs, k)
			}
		}
		d.auto = autostartEnabled()
		d.dashes = ctlListDashboards()
		if i.Active && i.ControllerUp {
			api := newAPI(i)
			d.mode = api.mode()
			if px, err := api.proxies(); err == nil {
				g := primarySelector(px)
				d.group = g
				if grp, ok := px[g]; ok {
					for _, n := range grp.All {
						if n == "DIRECT" || n == "REJECT" {
							continue
						}
						item := nodeItem{name: n, delay: -1, selected: n == grp.Now}
						if p, ok := px[n]; ok {
							item.delay = p.delay()
						}
						d.nodes = append(d.nodes, item)
					}
				}
			}
			if tr, err := api.traffic(); err == nil {
				d.tr = tr
			}
		}
		return d
	}
}

func startCmd() tea.Cmd {
	return func() tea.Msg {
		if _, err := runCtl("install-unit"); err != nil {
			return statusMsg("Ошибка юнита: " + err.Error())
		}
		if _, err := runCtl("start"); err != nil {
			return statusMsg("Не удалось включить: " + err.Error())
		}
		return statusMsg("Включено")
	}
}

func stopCmd() tea.Cmd {
	return func() tea.Msg {
		if _, err := runCtl("stop"); err != nil {
			return statusMsg("Не удалось выключить: " + err.Error())
		}
		return statusMsg("Выключено")
	}
}

func setModeCmd(i Info, m string) tea.Cmd {
	return func() tea.Msg {
		if err := newAPI(i).setMode(m); err != nil {
			return statusMsg("Режим: " + err.Error())
		}
		return statusMsg("Режим: " + m)
	}
}

func selectNodeCmd(i Info, group, name string) tea.Cmd {
	return func() tea.Msg {
		if err := newAPI(i).selectNode(group, name); err != nil {
			return statusMsg("Нода: " + err.Error())
		}
		return statusMsg("Нода: " + name)
	}
}

func pingCmd(i Info, group string) tea.Cmd {
	return func() tea.Msg {
		newAPI(i).groupDelay(group)
		return statusMsg("Пинг обновлён")
	}
}

func setDashboardCmd(name string) tea.Cmd {
	return func() tea.Msg {
		runCtl("set-dashboard", name)
		return statusMsg("Панель: " + name)
	}
}

func setSubCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if _, err := runCtl("set-sub", name); err != nil {
			return statusMsg("Подписка: " + err.Error())
		}
		return statusMsg("Подписка: " + name)
	}
}

func addSubCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if _, err := runCtl("add-sub", url); err != nil {
			return statusMsg("Добавление: " + err.Error())
		}
		return statusMsg("Подписка добавлена")
	}
}

func setAutoCmd(on bool) tea.Cmd {
	return func() tea.Msg {
		setAutostart(on)
		if on {
			return statusMsg("Авто-старт включён")
		}
		return statusMsg("Авто-старт выключен")
	}
}

// --- bubbletea -------------------------------------------------------------

func initialModel() model {
	cfg := loadTUIConfig()
	ti := textinput.New()
	ti.Placeholder = "https://rw.geekcom.org/api/sub/..."
	ti.CharLimit = 300
	ti.Width = innerW - 6
	z := []zone{}
	return model{
		th:     themeByName(cfg.Theme),
		screen: "main",
		ti:     ti,
		zones:  &z,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadInfoCmd(), checkUpdateCmd(), tick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(loadInfoCmd(), tick())

	case infoMsg:
		if msg.err == nil {
			m.info = msg.info
			m.api = newAPI(msg.info)
			return m, refreshCmd(msg.info)
		}
		m.status = "ctl: " + msg.err.Error()
		return m, nil

	case dataMsg:
		m.group = msg.group
		if len(msg.nodes) > 0 || m.info.Active {
			m.nodes = msg.nodes
		}
		m.mode = msg.mode
		m.tr = msg.tr
		m.subs = msg.subs
		m.current = msg.current
		m.dashes = msg.dashes
		m.autostrt = msg.auto
		return m, nil

	case updateMsg:
		m.latest = string(msg)
		return m, nil

	case statusMsg:
		m.status = string(msg)
		m.busy = ""
		return m, tea.Batch(loadInfoCmd())

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	if m.screen == "addsub" {
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.screen == "addsub" {
		switch msg.String() {
		case "esc":
			m.screen = "main"
			return m, nil
		case "enter":
			url := strings.TrimSpace(m.ti.Value())
			m.screen = "main"
			if url != "" {
				m.busy = "Добавляю..."
				return m, addSubCmd(url)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.screen = "main"
		return m, nil
	case "t":
		return m.toggleTheme()
	}
	return m, nil
}

func (m model) toggleTheme() (tea.Model, tea.Cmd) {
	m.th = otherTheme(m.th)
	saveTUIConfig(tuiConfig{Theme: m.th.Name})
	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button == tea.MouseButtonWheelDown {
		if m.nodeScr < len(m.nodes)-maxNodes {
			m.nodeScr++
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		if m.nodeScr > 0 {
			m.nodeScr--
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	for _, z := range *m.zones {
		if msg.X >= z.x0 && msg.X <= z.x1 && msg.Y >= z.y0 && msg.Y <= z.y1 {
			return m.activate(z)
		}
	}
	return m, nil
}

func (m model) activate(z zone) (tea.Model, tea.Cmd) {
	switch z.id {
	case "theme":
		return m.toggleTheme()
	case "toggle":
		if m.info.Active {
			m.busy = "Выключаю..."
			return m, stopCmd()
		}
		m.busy = "Включаю..."
		return m, startCmd()
	case "mode":
		return m, setModeCmd(m.info, z.data)
	case "node":
		return m, selectNodeCmd(m.info, m.group, z.data)
	case "ping":
		m.status = "Пингую..."
		return m, pingCmd(m.info, m.group)
	case "webpanel":
		// нет выбранной панели — открываем выбор, иначе сразу панель
		if m.info.Dashboard == "" {
			m.screen = "dashpick"
			return m, nil
		}
		openURL(dashURL(m.info, m.info.Dashboard))
		m.status = "Открываю: " + m.info.Dashboard
		return m, nil
	case "dashpick":
		m.screen = "dashpick"
		return m, nil
	case "dashsel":
		m.screen = "main"
		m.info.Dashboard = z.data
		openURL(dashURL(m.info, z.data))
		return m, setDashboardCmd(z.data)
	case "logs":
		openLogs()
		m.status = "Логи в Konsole"
		return m, nil
	case "openurl":
		openURL(z.data)
		m.status = "Открываю ссылку в браузере"
		return m, nil
	case "appupdate":
		openInstaller()
		m.status = "Обновление запущено в новом окне"
		return m, nil
	case "addsub":
		m.screen = "addsub"
		m.ti.SetValue("")
		m.ti.Focus()
		return m, textinput.Blink
	case "subpick":
		m.screen = "subpick"
		return m, nil
	case "subsel":
		m.screen = "main"
		return m, setSubCmd(z.data)
	case "update":
		if m.current != "" {
			m.status = "Обновляю подписку..."
			return m, func() tea.Msg {
				runCtl("update-sub", m.current)
				return statusMsg("Подписка обновлена")
			}
		}
		return m, nil
	case "autostart":
		return m, setAutoCmd(!m.autostrt)
	case "paste":
		m.ti.SetValue(clipboardPaste())
		return m, nil
	case "addok":
		url := strings.TrimSpace(m.ti.Value())
		m.screen = "main"
		if url != "" {
			m.busy = "Добавляю..."
			return m, addSubCmd(url)
		}
		return m, nil
	case "cancel":
		m.screen = "main"
		return m, nil
	}
	return m, nil
}

func main() {
	preview := flag.String("preview", "", "render one frame to stdout (dracula|macchiato) and exit")
	showVer := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVer {
		fmt.Println(version)
		return
	}
	if *preview != "" {
		m := initialModel()
		m.th = themeByName(*preview)
		m.w, m.h = 80, 34
		version = "1.1.0"
		m.latest = "v1.1.1"
		m.info = Info{Active: true, ControllerPort: 9090, Current: "Geekcom", Dashboard: "metacubexd"}
		m.mode = "rule"
		m.current = "Geekcom"
		m.subs = []string{"Geekcom"}
		m.dashes = []string{"metacubexd", "yacd", "zashboard"}
		m.tr = Traffic{Up: 1200000, Down: 8400000}
		m.group = "→ Remnawave"
		m.nodes = []nodeItem{
			{"PL-VPS", 42, true}, {"BG-VPS", 58, false}, {"LT-VPS", 61, false},
			{"SE-VPS", 70, false}, {"US-VPS", 140, false}, {"⚡ AUTO", -1, false},
		}
		fmt.Print(m.View())
		return
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

var _ = lipgloss.Width
