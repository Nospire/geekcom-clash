package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

type btnSpec struct {
	label, id, data string
	active          bool
}

type builder struct {
	m     model
	base  lipgloss.Style
	lines []string
	zones []zone
}

func (b *builder) line(s string) { b.lines = append(b.lines, s) }
func (b *builder) blank()        { b.lines = append(b.lines, b.base.Render("")) }
func (b *builder) y() int        { return len(b.lines) }

func (b *builder) addZone(id, data string, x0, x1, y0, y1 int) {
	b.zones = append(b.zones, zone{x0: x0, y0: y0, x1: x1, y1: y1, id: id, data: data})
}

// buttonsRow рендерит ряд кнопок с indent и регистрирует зоны на текущей строке.
func (b *builder) buttonsRow(indent int, cells []btnSpec) {
	t := b.m.th
	y := b.y()
	cur := indent
	out := b.base.Render(repeat(" ", indent))
	for _, c := range cells {
		st := lipgloss.NewStyle().Background(t.BtnBg).Foreground(t.BtnFg)
		if c.active {
			st = lipgloss.NewStyle().Background(t.Accent).Foreground(t.Bg).Bold(true)
		}
		cell := st.Render(" " + c.label + " ")
		w := lipgloss.Width(cell)
		b.addZone(c.id, c.data, cur, cur+w-1, y, y)
		out += cell + b.base.Render("  ")
		cur += w + 2
	}
	b.line(out)
}

func repeat(s string, n int) string {
	r := ""
	for i := 0; i < n; i++ {
		r += s
	}
	return r
}

func fmtRate(v int64) string {
	f := float64(v)
	u := []string{"B", "KB", "MB", "GB"}
	i := 0
	for f >= 1024 && i < len(u)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s/s", f, u[i])
	}
	return fmt.Sprintf("%.1f %s/s", f, u[i])
}

func (m model) View() string {
	t := m.th
	w := m.w
	if w < innerW+2 {
		w = innerW + 2
	}
	h := m.h
	if h < 24 {
		h = 24
	}
	base := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Fg)
	b := &builder{m: m, base: base}

	title := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Accent).Bold(true).Render(" Geekcom Clash") +
		lipgloss.NewStyle().Background(t.Bg).Foreground(t.Muted).Render(" v"+normVer(version))
	gear := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Muted).Render(" ⚙ ")
	pad := innerW - lipgloss.Width(title) - lipgloss.Width(gear)
	if pad < 1 {
		pad = 1
	}
	b.addZone("theme", "", innerW-3, innerW, 0, 0)
	b.line(title + base.Render(repeat(" ", pad)) + gear)
	b.blank()

	switch m.screen {
	case "addsub":
		m.renderAddSub(b)
	case "subpick":
		m.renderSubPick(b)
	case "dashpick":
		m.renderDashPick(b)
	default:
		m.renderMain(b)
	}

	// статус/занятость
	b.blank()
	msg := m.status
	if m.busy != "" {
		msg = m.busy
	}
	if msg != "" {
		b.line(lipgloss.NewStyle().Background(t.Bg).Foreground(t.Warn).Render(" " + msg))
	}

	// Прокрутка: если экран не влезает в окно (много подписок/нод, маленькое
	// окно Konsole), показываем «окно» высотой viewH начиная со scroll и
	// полосу прокрутки справа. Без этого bubbletea молча срезал бы строки.
	viewH := h - 1 // последняя строка — подсказка
	m.lay.content, m.lay.view = len(b.lines), viewH
	scroll := m.scroll
	if mx := m.lay.maxScroll(); scroll > mx {
		scroll = mx
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + viewH
	if end > len(b.lines) {
		end = len(b.lines)
	}

	// зоны кликов — в координатах видимого окна; невидимые отбрасываем
	var zones []zone
	for _, z := range b.zones {
		z.y0 -= scroll
		z.y1 -= scroll
		if z.y1 < 0 || z.y0 >= viewH {
			continue
		}
		if z.y0 < 0 {
			z.y0 = 0
		}
		if z.y1 >= viewH {
			z.y1 = viewH - 1
		}
		zones = append(zones, z)
	}
	*m.zones = zones

	// собрать полотно: каждую строку дотянуть до ширины фоном, добить высоту
	rowStyle := lipgloss.NewStyle().Background(t.Bg).Width(w - 1)
	bar := scrollbar(t, viewH, len(b.lines), scroll)
	var canvas []string
	for i := 0; i < viewH; i++ {
		ln := ""
		if scroll+i < end {
			ln = b.lines[scroll+i]
		}
		canvas = append(canvas, rowStyle.Render(ln)+bar[i])
	}
	hint := " тык мышью/пальцем · t — тема · текст: Steam+X"
	if m.lay.maxScroll() > 0 {
		hint = " колесо/↑↓ — прокрутка · t — тема · текст: Steam+X"
	}
	foot := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Muted).Width(w).
		Render(hint)
	canvas = append(canvas, foot)
	return lipgloss.JoinVertical(lipgloss.Left, canvas...)
}

// scrollbar — колонка шириной 1 символ: трек + ползунок. Пустая, если всё
// влезает.
func scrollbar(t Theme, viewH, content, scroll int) []string {
	out := make([]string, viewH)
	blank := lipgloss.NewStyle().Background(t.Bg).Render(" ")
	if content <= viewH || viewH <= 0 {
		for i := range out {
			out[i] = blank
		}
		return out
	}
	thumb := viewH * viewH / content
	if thumb < 1 {
		thumb = 1
	}
	pos := scroll * (viewH - thumb) / (content - viewH)
	track := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Border).Render("│")
	knob := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Accent).Render("█")
	for i := range out {
		if i >= pos && i < pos+thumb {
			out[i] = knob
		} else {
			out[i] = track
		}
	}
	return out
}

func (m model) renderMain(b *builder) {
	t := m.th
	base := b.base
	// статус
	dot := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Off).Render("○")
	stTxt := "ВЫКЛЮЧЕН"
	if m.info.Active {
		dot = lipgloss.NewStyle().Background(t.Bg).Foreground(t.On).Render("●")
		via := ""
		for _, n := range m.nodes {
			if n.selected {
				via = "  через  " + n.name
			}
		}
		stTxt = "ВКЛЮЧЁН" + via
	}
	b.line(base.Render(" ") + dot + base.Render(" ") +
		lipgloss.NewStyle().Background(t.Bg).Foreground(t.Fg).Bold(true).Render(stTxt))
	b.blank()

	// доступно обновление
	if m.updateAvailable() {
		b.line(lipgloss.NewStyle().Background(t.Bg).Foreground(t.Warn).
			Render(" ⬆ Доступно обновление " + m.latest))
		b.buttonsRow(2, []btnSpec{{label: "⬆ Обновить", id: "appupdate", active: true}})
		b.blank()
	}

	// большая кнопка вкл/выкл
	label := "⏻   ВКЛЮЧИТЬ"
	bg := t.On
	if m.info.Active {
		label = "⏻   ВЫКЛЮЧИТЬ"
		bg = t.Off
	}
	btn := lipgloss.NewStyle().Background(bg).Foreground(t.Bg).Bold(true).
		Width(innerW - 4).Align(lipgloss.Center).Padding(1, 0)
	box := btn.Render(label)
	y := b.y()
	for i, ln := range splitLines(box) {
		b.line(base.Render("  ") + ln)
		_ = i
	}
	b.addZone("toggle", "", 2, innerW-2, y, b.y()-1)
	b.blank()

	if m.info.Active {
		// трафик
		tr := fmt.Sprintf("   ↑ %s      ↓ %s", fmtRate(m.tr.Up), fmtRate(m.tr.Down))
		b.line(lipgloss.NewStyle().Background(t.Bg).Foreground(t.Accent).Render(tr))
		b.blank()
	}

	// подписка
	subName := m.current
	if subName == "" {
		subName = "— нет —"
	}
	b.line(base.Render(" Подписка"))
	b.buttonsRow(2, []btnSpec{
		{label: subName + " ▾", id: "subpick"},
		{label: "⟳ Обновить", id: "update"},
		{label: "＋ Подписка", id: "addsub"},
	})
	b.blank()

	// веб-импорт с телефона
	impLbl := " ВЫКЛ "
	if m.importer != nil {
		impLbl = " ВКЛ "
	}
	b.line(base.Render(" Веб-импорт (с телефона)"))
	b.buttonsRow(2, []btnSpec{{label: impLbl, id: "webimport", active: m.importer != nil}})
	if m.impURL != "" {
		b.line(lipgloss.NewStyle().Background(t.Bg).Foreground(t.Accent).Render("   " + m.impURL))
	}
	b.blank()

	if m.info.Active {
		// ноды
		b.line(base.Render(" Нода") + lipgloss.NewStyle().Background(t.Bg).Foreground(t.Muted).Render("   (тык — выбрать)") )
		b.buttonsRow(2, []btnSpec{{label: "⚡ Проверить задержку", id: "ping"}})
		for _, n := range m.nodes {
			y := b.y()
			rowSt := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Fg)
			if n.selected {
				rowSt = lipgloss.NewStyle().Background(t.SelBg).Foreground(t.Fg).Bold(true)
			}
			d := "    —"
			if n.delay == 0 {
				d = "таймаут" // mihomo пишет 0 в историю, если нода не ответила
			} else if n.delay > 0 {
				d = fmt.Sprintf("%4d ms", n.delay)
			}
			txt := fmt.Sprintf(" %s%-28s %8s", markPlain(n.selected), n.name, d)
			b.line(base.Render(" ") + rowSt.Width(innerW-2).Render(txt))
			b.addZone("node", n.name, 0, innerW, y, y)
		}
		b.blank()
	}

	// режим — виден и при выключенном VPN: выбор запоминается и применяется
	// при следующем включении
	mode := m.mode
	if mode == "" {
		mode = m.info.Mode
	}
	b.line(base.Render(" Режим") + lipgloss.NewStyle().Background(t.Bg).Foreground(t.Muted).
		Render("   (запоминается)"))
	b.buttonsRow(2, []btnSpec{
		{label: "Правила", id: "mode", data: "rule", active: mode == "rule"},
		{label: "Глобально", id: "mode", data: "global", active: mode == "global"},
		{label: "Напрямую", id: "mode", data: "direct", active: mode == "direct"},
	})
	b.blank()

	// веб-панель: выбор + открытие
	dashName := m.info.Dashboard
	if dashName == "" {
		dashName = "выбрать"
	}
	b.line(base.Render(" Веб-панель"))
	b.buttonsRow(2, []btnSpec{
		{label: dashName + " ▾", id: "dashpick"},
		{label: "🌐 Открыть", id: "webpanel"},
		{label: "📜 Логи", id: "logs"},
	})
	b.blank()

	// автозапуск
	autoLbl := " ВЫКЛ "
	if m.autostrt {
		autoLbl = " ВКЛ "
	}
	b.line(base.Render(" Авто-старт при загрузке"))
	b.buttonsRow(2, []btnSpec{{label: autoLbl, id: "autostart", active: m.autostrt}})
	b.blank()

	// сообщество
	b.line(base.Render(" Сообщество"))
	b.buttonsRow(2, []btnSpec{
		{label: "❤ Boosty", id: "openurl", data: linkBoosty},
		{label: "✈ Канал", id: "openurl", data: linkTGGames},
		{label: "✈ Новости", id: "openurl", data: linkTGNews},
		{label: "✈ Чат", id: "openurl", data: linkTGChat},
	})
}

func markPlain(sel bool) string {
	if sel {
		return "● "
	}
	return "  "
}

func (m model) renderSubPick(b *builder) {
	t := m.th
	b.line(b.base.Render(" Выбери подписку:"))
	b.blank()
	for _, s := range m.subs {
		b.buttonsRow(2, []btnSpec{{label: s, id: "subsel", data: s, active: s == m.current}})
	}
	b.blank()
	b.buttonsRow(2, []btnSpec{{label: "Отмена", id: "cancel"}})
	_ = t
}

func (m model) renderDashPick(b *builder) {
	t := m.th
	b.line(b.base.Render(" Выбери веб-панель:"))
	b.blank()
	if len(m.dashes) == 0 {
		b.line(lipgloss.NewStyle().Background(t.Bg).Foreground(t.Muted).
			Render(" нет установленных панелей (поставь через установщик)"))
	}
	for _, d := range m.dashes {
		b.buttonsRow(2, []btnSpec{{label: d, id: "dashsel", data: d, active: d == m.info.Dashboard}})
	}
	b.blank()
	b.buttonsRow(2, []btnSpec{{label: "Отмена", id: "cancel"}})
}

func (m model) renderAddSub(b *builder) {
	t := m.th
	muted := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Muted)
	b.line(b.base.Render(" Новая подписка — вставь ссылку:"))
	b.line(muted.Render(" (текст — экранная клавиатура Steam+X)"))
	b.line(muted.Render(" Можно: ссылку на подписку Clash/Mihomo (https://…),"))
	b.line(muted.Render(" base64-подписку v2ray или ссылку на сервер:"))
	b.line(muted.Render(" vless:// vmess:// ss:// trojan:// hysteria2:// (hy2://)"))
	b.blank()
	field := lipgloss.NewStyle().Background(t.Panel).Foreground(t.Fg).
		Width(innerW - 4).Padding(0, 1).Render(m.ti.View())
	b.line(b.base.Render("  ") + field)
	b.blank()
	b.buttonsRow(2, []btnSpec{
		{label: "📋 Вставить", id: "paste"},
		{label: "ОК", id: "addok", active: true},
		{label: "Отмена", id: "cancel"},
	})
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, cur)
	return out
}
