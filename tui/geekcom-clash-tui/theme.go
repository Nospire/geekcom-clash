package main

import "github.com/charmbracelet/lipgloss"

// Theme — набор цветов для интерфейса.
type Theme struct {
	Name   string
	Bg     lipgloss.Color // фон полотна
	Panel  lipgloss.Color // фон панелей/боксов
	Border lipgloss.Color // рамки
	Fg     lipgloss.Color // основной текст
	Muted  lipgloss.Color // приглушённый текст
	Accent lipgloss.Color // акцент (заголовки, выбранное)
	On     lipgloss.Color // включено / успех
	Off    lipgloss.Color // выключено / ошибка
	Warn   lipgloss.Color // предупреждение
	SelBg  lipgloss.Color // фон выделенной строки
	BtnBg  lipgloss.Color // фон кнопки
	BtnFg  lipgloss.Color // текст кнопки
}

// Dracula — канонная тёмная палитра.
var Dracula = Theme{
	Name:   "dracula",
	Bg:     "#282a36",
	Panel:  "#21222c",
	Border: "#44475a",
	Fg:     "#f8f8f2",
	Muted:  "#6272a4",
	Accent: "#bd93f9",
	On:     "#50fa7b",
	Off:    "#ff5555",
	Warn:   "#f1fa8c",
	SelBg:  "#44475a",
	BtnBg:  "#44475a",
	BtnFg:  "#f8f8f2",
}

// Macchiato (Мокачино) — тёплая светлая палитра (кофе со сливками).
var Macchiato = Theme{
	Name:   "macchiato",
	Bg:     "#f3ebe1",
	Panel:  "#eaddcd",
	Border: "#d6c3a8",
	Fg:     "#4a3b30",
	Muted:  "#9b8266",
	Accent: "#a4632e",
	On:     "#5c843f",
	Off:    "#b4452f",
	Warn:   "#b8860b",
	SelBg:  "#e0cfb6",
	BtnBg:  "#dcc7a8",
	BtnFg:  "#4a3b30",
}

func themeByName(n string) Theme {
	if n == "macchiato" {
		return Macchiato
	}
	return Dracula
}

func otherTheme(t Theme) Theme {
	if t.Name == "dracula" {
		return Macchiato
	}
	return Dracula
}
