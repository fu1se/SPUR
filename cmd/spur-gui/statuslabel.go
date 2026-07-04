package main

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// newStatusLabel creates a label whose color setStatus will drive via
// widget.Label's Importance field — mirrors the Android app's
// StatusText composable (see Components.kt), just implemented against
// Fyne's built-in importance levels instead of a hand-picked Color call,
// so it automatically follows spurTheme's ColorNameError/ColorNameWarning/
// ColorNameSuccess instead of hardcoding colors a second time here.
func newStatusLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// setStatus classifies text by the same handful of substrings every
// status string in this app already produces (RU and EN both — the GUI
// is bilingual, see i18n.go) and colors the label accordingly: one
// shared rule instead of every tab re-deciding its own success/error/
// idle colors, same reasoning as Android's StatusText doc comment.
func setStatus(l *widget.Label, text string) {
	l.Text = text
	l.Importance = statusImportance(text)
	l.Refresh()
}

func statusImportance(text string) widget.Importance {
	lower := strings.ToLower(text)
	switch {
	case text == "":
		return widget.MediumImportance
	case strings.Contains(lower, "ошибка"), strings.Contains(lower, "error"),
		strings.Contains(lower, "отказано"):
		return widget.DangerImportance
	case strings.Contains(lower, "остановлено"), strings.Contains(lower, "stopped"),
		strings.Contains(lower, "не запущено"), strings.Contains(lower, "not running"),
		strings.Contains(lower, "не подключено"), strings.Contains(lower, "not connected"):
		return widget.LowImportance
	case strings.HasSuffix(text, "..."):
		return widget.WarningImportance
	default:
		return widget.SuccessImportance
	}
}
