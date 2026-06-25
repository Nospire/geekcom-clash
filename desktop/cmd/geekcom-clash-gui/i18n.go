package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

var lang = "ru"

func loadLang() { lang = fyne.CurrentApp().Preferences().StringWithFallback("lang", "ru") }
func setLang(l string) {
	lang = l
	fyne.CurrentApp().Preferences().SetString("lang", l)
}

func tr(key string) string {
	if m, ok := langData[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if v, ok := langData["ru"][key]; ok {
		return v
	}
	return key
}

// langSelect — выпадашка языка; onChange пересобирает текущий экран.
func langSelect(onChange func()) *widget.Select {
	codes := map[string]string{"Русский": "ru", "English": "en", "中文": "zh"}
	rev := map[string]string{"ru": "Русский", "en": "English", "zh": "中文"}
	s := widget.NewSelect([]string{"Русский", "English", "中文"}, nil)
	s.SetSelected(rev[lang]) // до назначения OnChanged — иначе рекурсия через rebuild
	s.OnChanged = func(v string) {
		if codes[v] == "" || codes[v] == lang {
			return
		}
		setLang(codes[v])
		onChange()
	}
	return s
}

var langData = map[string]map[string]string{
	"ru": {
		"welcome_title": "Привет! Это Geekcom Clash",
		"welcome_sub":   "Подписок ещё нет. Добавь первую — и можно подключаться.",
		"paste_link":    "Вставить ссылку",
		"phone_import":  "Импорт с телефона",
		"add_sub":       "Добавить подписку",
		"add":           "Добавить",
		"cancel":        "Отмена",
		"close":         "Закрыть",
		"scan_or_open":  "Отсканируй телефоном или открой адрес:",
		"paste_there":   "Вставь там ссылку — подписка добавится сюда автоматически.",
		"back":          "Назад",
		"settings":      "Настройки",
		"language":      "Язык",
		"autostart":     "Автозапуск VPN при входе",
		"remote_access": "Доступ к панели по сети",
		"dns_mode":      "Режим DNS",
		"dashboard":     "Веб-панель",
		"community":     "Сообщество",
		"plugin_ver":    "Плагин 1.3.0",
		"engine_line":   "Движок: geekcom-clash · nightly",
		"link_channel":  "Канал",
		"link_news":     "Новости",
		"link_chat":     "Чат Geekcom-HUB",
		"connected":     "●  Подключено",
		"disconnected":  "○  Отключено",
		"routing_mode":  "Режим маршрутизации",
		"mode_rule":     "Правила",
		"mode_global":   "Глобально",
		"mode_direct":   "Напрямую",
		"subscriptions": "Подписки",
		"servers":       "Серверы",
		"auto":          "Авто (быстрейшая)",
		"server_prefix": "Сервер: ",
		"connect_hint":  " · подключите VPN",
		"connect_prompt": "Подключите VPN",
		"del_sub":       "Удалить подписку",
		"del_confirm":   "Удалить «%s»?",
	},
	"en": {
		"welcome_title": "Welcome to Geekcom Clash",
		"welcome_sub":   "No subscriptions yet. Add one to get connected.",
		"paste_link":    "Paste link",
		"phone_import":  "Import from phone",
		"add_sub":       "Add subscription",
		"add":           "Add",
		"cancel":        "Cancel",
		"close":         "Close",
		"scan_or_open":  "Scan with your phone or open the address:",
		"paste_there":   "Paste a link there — it will be added here automatically.",
		"back":          "Back",
		"settings":      "Settings",
		"language":      "Language",
		"autostart":     "Auto-start VPN on login",
		"remote_access": "Allow panel access over LAN",
		"dns_mode":      "DNS mode",
		"dashboard":     "Web panel",
		"community":     "Community",
		"plugin_ver":    "Plugin 1.3.0",
		"engine_line":   "Engine: geekcom-clash · nightly",
		"link_channel":  "Channel",
		"link_news":     "News",
		"link_chat":     "Geekcom-HUB chat",
		"connected":     "●  Connected",
		"disconnected":  "○  Disconnected",
		"routing_mode":  "Routing mode",
		"mode_rule":     "Rule",
		"mode_global":   "Global",
		"mode_direct":   "Direct",
		"subscriptions": "Subscriptions",
		"servers":       "Servers",
		"auto":          "Auto (fastest)",
		"server_prefix": "Server: ",
		"connect_hint":  " · connect VPN",
		"connect_prompt": "Connect VPN",
		"del_sub":       "Delete subscription",
		"del_confirm":   "Delete «%s»?",
	},
	"zh": {
		"welcome_title": "欢迎使用 Geekcom Clash",
		"welcome_sub":   "还没有订阅。添加一个即可连接。",
		"paste_link":    "粘贴链接",
		"phone_import":  "从手机导入",
		"add_sub":       "添加订阅",
		"add":           "添加",
		"cancel":        "取消",
		"close":         "关闭",
		"scan_or_open":  "用手机扫描或打开地址：",
		"paste_there":   "在那里粘贴链接 — 会自动添加到这里。",
		"back":          "返回",
		"settings":      "设置",
		"language":      "语言",
		"autostart":     "登录时自动启动 VPN",
		"remote_access": "允许局域网访问面板",
		"dns_mode":      "DNS 模式",
		"dashboard":     "网页面板",
		"community":     "社区",
		"plugin_ver":    "插件 1.3.0",
		"engine_line":   "引擎：geekcom-clash · nightly",
		"link_channel":  "频道",
		"link_news":     "新闻",
		"link_chat":     "Geekcom-HUB 聊天",
		"connected":     "●  已连接",
		"disconnected":  "○  已断开",
		"routing_mode":  "路由模式",
		"mode_rule":     "规则",
		"mode_global":   "全局",
		"mode_direct":   "直连",
		"subscriptions": "订阅",
		"servers":       "服务器",
		"auto":          "自动（最快）",
		"server_prefix": "服务器：",
		"connect_hint":  " · 请连接 VPN",
		"connect_prompt": "请连接 VPN",
		"del_sub":       "删除订阅",
		"del_confirm":   "删除「%s」？",
	},
}
