// geekcom-clash-gui — десктоп-GUI (Fyne) в стиле Steam.
// Базовое окно: приветствие (нет подписок) ↔ главное (подключение + серверы +
// режим + подписка). Всё на встроенном движке (service/subscription/config) и
// API ядра (clashapi). Кастомные виджеты — widgets.go.
package main

import (
	_ "embed"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/skip2/go-qrcode"
	"gopkg.in/yaml.v3"

	"geekcom-clash/internal/clashapi"
	"geekcom-clash/internal/config"
	"geekcom-clash/internal/paths"
	"geekcom-clash/internal/service"
	"geekcom-clash/internal/subscription"
	"geekcom-clash/internal/webimport"
)

//go:embed logo.png
var logoBytes []byte
var logoRes = fyne.NewStaticResource("logo.png", logoBytes)

var (
	cBlue   = color.NRGBA{0x1a, 0x9f, 0xff, 0xff}
	cInfo   = color.NRGBA{0x66, 0xc0, 0xf4, 0xff}
	cGreen  = color.NRGBA{0x5b, 0xa3, 0x2b, 0xff}
	cGreenT = color.NRGBA{0x8b, 0xc3, 0x4a, 0xff}
	cAmber  = color.NRGBA{0xe8, 0xb8, 0x4b, 0xff}
	cMuted  = color.NRGBA{0x8f, 0x98, 0xa0, 0xff}
	cWhite  = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	cPanel  = color.NRGBA{0x1b, 0x28, 0x38, 0xff}
	cRow    = color.NRGBA{0x1b, 0x28, 0x38, 0xff}
	cSelBg  = color.NRGBA{0x1a, 0x9f, 0xff, 0x2a}
	cRail   = color.NRGBA{0x13, 0x16, 0x1c, 0xff}
)

type srv struct {
	name string
	ping int
}

func main() {
	a := app.NewWithID("org.geekcom.clash")
	loadLang()
	a.Settings().SetTheme(steamTheme{})
	w := a.NewWindow("Geekcom Clash")
	w.SetIcon(logoRes)
	w.Resize(fyne.NewSize(920, 540))

	var show func()
	var curStop func()
	show = func() {
		if curStop != nil {
			curStop()
			curStop = nil
		}
		c, _ := config.Load()
		if len(c.Subscriptions) == 0 {
			w.SetContent(buildWelcome(w, show))
		} else {
			content, start, stop := buildMain(w, show)
			curStop = stop
			w.SetContent(content)
			start()
		}
	}
	show()
	w.ShowAndRun()
}

// ── Приветствие (нет подписок) ──
func buildWelcome(w fyne.Window, onChanged func()) fyne.CanvasObject {
	cat := canvas.NewImageFromResource(logoRes)
	cat.FillMode = canvas.ImageFillContain
	catBox := container.NewGridWrap(fyne.NewSize(96, 96), cat)

	title := newText(tr("welcome_title"), cWhite, 22, true)
	title.Alignment = fyne.TextAlignCenter
	sub := newText(tr("welcome_sub"), cMuted, 14, false)
	sub.Alignment = fyne.TextAlignCenter

	paste := widget.NewButtonWithIcon(tr("paste_link"), theme.ContentPasteIcon(), func() {
		askLink(w, onChanged)
	})
	paste.Importance = widget.HighImportance
	phone := widget.NewButtonWithIcon(tr("phone_import"), theme.ComputerIcon(), func() {
		phoneImport(w, onChanged)
	})
	btns := container.NewHBox(layout.NewSpacer(), paste, phone, layout.NewSpacer())

	langRow := container.NewHBox(layout.NewSpacer(), langSelect(onChanged))
	return container.NewBorder(container.NewPadded(langRow), nil, nil, nil,
		container.NewCenter(container.NewVBox(
			container.NewCenter(catBox),
			title, sub,
			gap(8),
			btns,
		)))
}

func askLink(w fyne.Window, onChanged func()) {
	entry := widget.NewMultiLineEntry()
	entry.SetPlaceHolder("vless://…  или  https://…/sub")
	entry.SetMinRowsVisible(3)
	d := dialog.NewCustomConfirm(tr("add_sub"), tr("add"), tr("cancel"), entry, func(ok bool) {
		if !ok {
			return
		}
		link := strings.TrimSpace(entry.Text)
		if link == "" {
			return
		}
		go func() {
			c, _ := config.Load()
			res, err := subscription.Add(link, c.Subscriptions)
			if err != nil {
				fyne.Do(func() { dialog.ShowError(err, w) })
				return
			}
			config.RegisterSub(res.Name, res.URL)
			fyne.Do(onChanged)
		}()
	}, w)
	d.Resize(fyne.NewSize(480, 240))
	d.Show()
}

func phoneImport(w fyne.Window, after func()) {
	link, stop, err := webimport.Start(func(name string) { fyne.Do(after) })
	if err != nil {
		dialog.ShowError(err, w)
		return
	}
	var qrObj fyne.CanvasObject = newText("", cMuted, 13, false)
	if png, e := qrcode.Encode(link, qrcode.Medium, 240); e == nil {
		img := canvas.NewImageFromResource(fyne.NewStaticResource("qr.png", png))
		img.FillMode = canvas.ImageFillContain
		qrObj = container.NewGridWrap(fyne.NewSize(220, 220), img)
	}
	body := container.NewVBox(
		newText(tr("scan_or_open"), cMuted, 13, false),
		container.NewCenter(qrObj),
		container.NewCenter(newText(link, cInfo, 16, true)),
		newText(tr("paste_there"), cMuted, 13, false),
	)
	d := dialog.NewCustom(tr("phone_import"), tr("close"), body, w)
	d.SetOnClosed(func() { stop() })
	d.Resize(fyne.NewSize(430, 430))
	d.Show()
}

func addChooser(w fyne.Window, after func()) {
	var d dialog.Dialog
	paste := widget.NewButtonWithIcon(tr("paste_link"), theme.ContentPasteIcon(),
		func() { d.Hide(); askLink(w, after) })
	paste.Importance = widget.HighImportance
	phone := widget.NewButtonWithIcon(tr("phone_import"), theme.ComputerIcon(),
		func() { d.Hide(); phoneImport(w, after) })
	d = dialog.NewCustom(tr("add_sub"), tr("cancel"), container.NewVBox(paste, phone), w)
	d.Show()
}

// ── Настройки ──
func buildSettings(w fyne.Window, back func()) fyne.CanvasObject {
	backBtn := widget.NewButtonWithIcon(tr("back"), theme.NavigateBackIcon(), back)

	autostart := settingToggle(tr("autostart"),
		func() bool { c, _ := config.Load(); return c.Autostart },
		func(v bool) { config.Set("autostart", v) })

	remote := settingToggle(tr("remote_access"),
		func() bool { c, _ := config.Load(); return c.AllowRemoteAccess },
		func(v bool) {
			go func() {
				config.Set("allow_remote_access", v)
				if service.IsActive() {
					service.Restart()
				}
			}()
		})

	dnsFake := segment("Fake-IP")
	dnsRedir := segment("Redir-Host")
	applyDNS := func() {
		c, _ := config.Load()
		setSeg(dnsFake, c.EnhancedMode == "fake-ip")
		setSeg(dnsRedir, c.EnhancedMode == "redir-host")
	}
	setDNS := func(m string) {
		go func() {
			config.Set("enhanced_mode", m)
			if service.IsActive() {
				service.Restart()
			}
			fyne.Do(applyDNS)
		}()
	}
	dnsFake.OnTap = func() { setDNS("fake-ip") }
	dnsRedir.OnTap = func() { setDNS("redir-host") }
	applyDNS()
	dnsRow := card(container.NewVBox(
		newText(tr("dns_mode"), cMuted, 12, false),
		container.NewGridWithColumns(2, dnsFake, dnsRedir)))

	dash := widget.NewSelect([]string{"metacubexd", "yacd"}, nil)
	if c, _ := config.Load(); c.Dashboard != "" {
		dash.SetSelected(c.Dashboard)
	}
	dash.OnChanged = func(v string) { // после SetSelected — иначе рестарт при открытии
		go func() {
			config.Set("dashboard", v)
			if service.IsActive() {
				service.Restart()
			}
		}()
	}
	dashRow := card(container.NewBorder(nil, nil,
		newText(tr("dashboard"), cWhite, 15, false), dash, layout.NewSpacer()))

	ver := card(container.NewBorder(nil, nil,
		newText(tr("plugin_ver"), cWhite, 14, false),
		newText(tr("engine_line"), cMuted, 12, false), layout.NewSpacer()))

	links := card(container.NewVBox(
		newText(tr("community"), cMuted, 12, false),
		container.NewGridWithColumns(2,
			widget.NewHyperlink("Boosty", mustURL("https://boosty.to/steamdecks")),
			widget.NewHyperlink(tr("link_channel"), mustURL("https://t.me/geekcom_deck_games")),
			widget.NewHyperlink(tr("link_news"), mustURL("https://t.me/geekcomdeck_news")),
			widget.NewHyperlink(tr("link_chat"), mustURL("https://t.me/Geekcom_hub")),
			widget.NewHyperlink("GitHub", mustURL("https://github.com/Nospire/geekcom-clash")))))

	langRow := card(container.NewBorder(nil, nil,
		newText(tr("language"), cWhite, 15, false),
		langSelect(func() { w.SetContent(buildSettings(w, back)) }), layout.NewSpacer()))

	body := container.NewVBox(langRow, autostart, remote, dnsRow, dashRow, ver, links)
	header := container.NewHBox(backBtn, layout.NewSpacer(), newText(tr("settings"), cWhite, 16, true))
	return container.NewBorder(container.NewPadded(header), nil, nil, nil,
		container.NewVScroll(container.NewPadded(body)))
}

func settingToggle(label string, get func() bool, set func(bool)) fyne.CanvasObject {
	t := newToggleSwitch()
	t.On = get()
	t.OnTap = func() {
		t.On = !t.On
		t.Refresh()
		set(t.On)
	}
	return card(container.NewBorder(nil, nil, newText(label, cWhite, 15, false),
		container.NewCenter(t), layout.NewSpacer()))
}

func mustURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

// ── Главное окно (есть подписка) ──
func buildMain(w fyne.Window, onChanged func()) (fyne.CanvasObject, func(), func()) {
	api := clashapi.New()

	ring := canvas.NewCircle(color.Transparent)
	ring.StrokeWidth = 4
	ring.StrokeColor = cMuted
	cat := canvas.NewImageFromResource(logoRes)
	cat.FillMode = canvas.ImageFillContain
	ringBox := container.NewGridWrap(fyne.NewSize(78, 78),
		container.NewStack(ring, container.NewCenter(
			container.NewGridWrap(fyne.NewSize(46, 46), cat))))

	statusBig := newText(tr("disconnected"), cMuted, 22, true)
	statusSub := newText("", cMuted, 13, false)
	toggle := newToggleSwitch()

	segRule := segment(tr("mode_rule"))
	segGlobal := segment(tr("mode_global"))
	segDirect := segment(tr("mode_direct"))

	serverList := container.NewVBox()
	var servers []srv
	var current string
	var refresh func()

	rebuildServers := func() {
		serverList.Objects = nil
		for _, s := range servers {
			s := s
			nt := newText(displayName(s.name), cWhite, 14, false)
			if s.name == current {
				nt.Color = cInfo
			}
			pt := newText("", cMuted, 13, false)
			if s.ping > 0 {
				pt.Text = fmt.Sprintf("%d ms", s.ping)
				if s.ping < 80 {
					pt.Color = cGreenT
				} else {
					pt.Color = cAmber
				}
			}
			fill := cRow
			if s.name == current {
				fill = cSelBg
			}
			row := newTapCard(container.NewBorder(nil, nil, nt, pt, layout.NewSpacer()), fill)
			row.OnTap = func() { go func() { api.Select(s.name); fyne.Do(refresh) }() }
			serverList.Add(row)
		}
		serverList.Refresh()
	}

	pingAll := func() {
		for i := range servers {
			servers[i].ping = api.Delay(servers[i].name)
		}
		fyne.Do(rebuildServers)
	}

	// refresh: ВСЁ блокирующее (service.IsActive — subprocess, api.* — HTTP с
	// таймаутом) делаем в горутине, в UI-поток отдаём только обновление виджетов.
	// Иначе ядро тормозит/недоступно → окно виснет.
	refresh = func() {
		go func() {
			connected := service.IsActive()
			mode := api.Mode()
			var list []srv
			var cur, subText string
			if connected {
				if g, err := api.Servers(); err == nil {
					cur = g.Now
					old := map[string]int{}
					for _, s := range servers {
						old[s.name] = s.ping
					}
					for _, n := range g.All {
						list = append(list, srv{name: n, ping: old[n]})
					}
					sort.SliceStable(list, func(i, j int) bool { return list[i].name == clashapi.AutoNode })
					subText = tr("server_prefix") + displayName(cur)
				}
			} else {
				names, c2 := offlineNodes()
				for _, n := range names {
					list = append(list, srv{name: n})
				}
				cur = c2
				if len(names) > 0 {
					subText = tr("server_prefix") + displayName(cur) + tr("connect_hint")
				} else {
					subText = tr("connect_prompt")
				}
			}
			fyne.Do(func() {
				toggle.On = connected
				toggle.Refresh()
				if connected {
					statusBig.Text = tr("connected")
					statusBig.Color = cGreenT
					ring.StrokeColor = cGreen
				} else {
					statusBig.Text = tr("disconnected")
					statusBig.Color = cMuted
					ring.StrokeColor = cMuted
				}
				statusBig.Refresh()
				ring.Refresh()
				setSeg(segRule, mode == "rule")
				setSeg(segGlobal, mode == "global")
				setSeg(segDirect, mode == "direct")
				servers = list
				current = cur
				statusSub.Text = subText
				statusSub.Refresh()
				rebuildServers()
				if connected {
					go pingAll()
				}
			})
		}()
	}

	toggle.OnTap = func() {
		statusBig.Text = "…"
		statusBig.Color = cMuted
		statusBig.Refresh()
		go func() {
			if service.IsActive() {
				service.Stop()
			} else {
				service.Start()
			}
			time.Sleep(900 * time.Millisecond)
			fyne.Do(refresh)
		}()
	}

	setMode := func(m string) { go func() { api.SetMode(m); fyne.Do(refresh) }() }
	segRule.OnTap = func() { setMode("rule") }
	segGlobal.OnTap = func() { setMode("global") }
	segDirect.OnTap = func() { setMode("direct") }

	subsBox := container.NewVBox()
	var rebuildSubs func()
	rebuildSubs = func() {
		c, _ := config.Load()
		names := make([]string, 0, len(c.Subscriptions))
		for n := range c.Subscriptions {
			names = append(names, n)
		}
		sort.Strings(names)
		subsBox.Objects = nil
		for _, name := range names {
			name := name
			nt := newText(name, cWhite, 14, false)
			fill := cRow
			if name == c.Current {
				nt.Color = cInfo
				fill = cSelBg
			}
			del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				dialog.ShowConfirm(tr("del_sub"), fmt.Sprintf(tr("del_confirm"), name), func(ok bool) {
					if !ok {
						return
					}
					go func() {
						config.RemoveSub(name)
						cc, _ := config.Load()
						if len(cc.Subscriptions) == 0 {
							fyne.Do(onChanged)
							return
						}
						fyne.Do(func() { rebuildSubs(); refresh() })
					}()
				}, w)
			})
			del.Importance = widget.LowImportance
			row := newTapCard(container.NewBorder(nil, nil, nt, del, layout.NewSpacer()), fill)
			row.OnTap = func() {
				go func() {
					config.SetCurrent(name)
					if service.IsActive() {
						service.Restart()
					}
					fyne.Do(func() { rebuildSubs(); refresh() })
				}()
			}
			subsBox.Add(row)
		}
		add := widget.NewButtonWithIcon(tr("add_sub"), theme.ContentAddIcon(), func() {
			addChooser(w, func() { fyne.Do(func() { rebuildSubs(); refresh() }) })
		})
		add.Importance = widget.HighImportance
		subsBox.Add(add)
		subsBox.Refresh()
	}

	connContent := container.NewBorder(nil, nil, ringBox, container.NewCenter(toggle),
		container.NewVBox(layout.NewSpacer(), statusBig, statusSub, layout.NewSpacer()))
	connCard := card(connContent)

	modeRow := container.NewGridWithColumns(3, segRule, segGlobal, segDirect)

	left := container.NewVBox(
		connCard,
		newText(tr("routing_mode"), cMuted, 12, false),
		modeRow,
		gap(8),
		newText(tr("subscriptions"), cMuted, 12, false),
		subsBox,
	)

	rightInner := container.NewBorder(
		container.NewPadded(newText(tr("servers"), cMuted, 13, false)), nil, nil, nil,
		container.NewVScroll(serverList))
	right := container.NewStack(roundRect(cRail), container.NewPadded(rightInner))

	split := container.NewHSplit(container.NewPadded(left), container.NewPadded(right))
	split.Offset = 0.56

	logo := canvas.NewImageFromResource(logoRes)
	logo.FillMode = canvas.ImageFillContain
	logoBox := container.NewGridWrap(fyne.NewSize(24, 24), logo)
	gear := widget.NewButtonWithIcon("", theme.SettingsIcon(), nil)
	appbar := container.NewBorder(nil, nil,
		container.NewHBox(logoBox, newText("Geekcom Clash", cWhite, 15, false)),
		gear, layout.NewSpacer())
	root := container.NewBorder(container.NewPadded(appbar), nil, nil, nil, split)

	gear.OnTapped = func() {
		w.SetContent(buildSettings(w, func() { w.SetContent(root) }))
	}

	quit := make(chan struct{})
	start := func() {
		rebuildSubs()
		go func() {
			t := time.NewTicker(4 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-quit:
					return
				case <-t.C:
					refresh()
				}
			}
		}()
		refresh()
	}
	stop := func() {
		select {
		case <-quit:
		default:
			close(quit)
		}
	}
	return root, start, stop
}

// ── helpers ──
func displayName(n string) string {
	if n == clashapi.AutoNode {
		return tr("auto")
	}
	return n
}

func offlineNodes() ([]string, string) {
	c, _ := config.Load()
	if c.Current == "" {
		return nil, ""
	}
	data, err := os.ReadFile(paths.SubPath(c.Current))
	if err != nil {
		return nil, ""
	}
	var doc struct {
		Proxies []struct {
			Name string `yaml:"name"`
		} `yaml:"proxies"`
	}
	yaml.Unmarshal(data, &doc)
	names := []string{clashapi.AutoNode}
	for _, p := range doc.Proxies {
		if p.Name != "" {
			names = append(names, p.Name)
		}
	}
	cur := c.CurrentNode
	if cur == "" {
		cur = clashapi.AutoNode
	}
	return names, cur
}

func gap(h float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(0, h))
	return r
}

func newText(s string, c color.Color, size float32, bold bool) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Bold: bold}
	return t
}

func segment(label string) *tapCard {
	t := newText(label, cMuted, 14, false)
	t.Alignment = fyne.TextAlignCenter
	return newTapCard(container.NewCenter(t), cPanel)
}

func setSeg(s *tapCard, active bool) {
	t := segText(s)
	if active {
		s.SetFill(cBlue)
		if t != nil {
			t.Color = color.NRGBA{0x06, 0x12, 0x1f, 0xff}
			t.Refresh()
		}
	} else {
		s.SetFill(cPanel)
		if t != nil {
			t.Color = cMuted
			t.Refresh()
		}
	}
}

func segText(s *tapCard) *canvas.Text {
	if cen, ok := s.content.(*fyne.Container); ok && len(cen.Objects) > 0 {
		if t, ok := cen.Objects[0].(*canvas.Text); ok {
			return t
		}
	}
	return nil
}

func roundRect(c color.Color) *canvas.Rectangle {
	r := canvas.NewRectangle(c)
	r.CornerRadius = 12
	return r
}

func card(content fyne.CanvasObject) fyne.CanvasObject {
	r := canvas.NewRectangle(cPanel)
	r.CornerRadius = 12
	return container.NewStack(r, container.NewPadded(content))
}

type steamTheme struct{}

var _ fyne.Theme = steamTheme{}

func (steamTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return color.NRGBA{0x17, 0x1a, 0x21, 0xff}
	case theme.ColorNameForeground:
		return color.NRGBA{0xc6, 0xd4, 0xdf, 0xff}
	case theme.ColorNameButton, theme.ColorNameInputBackground:
		return cPanel
	case theme.ColorNameDisabled, theme.ColorNameDisabledButton:
		return color.NRGBA{0x2f, 0x39, 0x47, 0xff}
	case theme.ColorNamePrimary:
		return cBlue
	case theme.ColorNameHover:
		return color.NRGBA{0x24, 0x35, 0x4a, 0xff}
	case theme.ColorNameSeparator:
		return color.NRGBA{0x0a, 0x0d, 0x12, 0xff}
	case theme.ColorNamePlaceHolder:
		return cMuted
	case theme.ColorNameScrollBar:
		return color.NRGBA{0x2a, 0x3a, 0x52, 0xff}
	case theme.ColorNameSelection:
		return color.NRGBA{0x1a, 0x9f, 0xff, 0x33}
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (steamTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (steamTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }

// Size — Steam-плотность под палец: тонкий скроллбар, скруглённые инпуты под
// карточки, чуть крупнее текст и базовый паддинг.
func (steamTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNamePadding:
		return 5
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameText:
		return 15
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 9
	}
	return theme.DefaultTheme().Size(n)
}
