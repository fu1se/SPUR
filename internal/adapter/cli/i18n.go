package cli

import "strings"

// Lang is a UI language spur's CLI can speak. New languages are added by
// extending the catalog type (catalog.go) with a translation and this
// list — nothing else in the package hardcodes "ru"/"en" beyond here.
type Lang string

const (
	LangRU Lang = "ru"
	LangEN Lang = "en"
)

// currentLang is package-level state set once, at startup, before any
// command is built or run — the same pattern the "version" package var
// already uses (see root.go). Every user-facing string in this package
// goes through msg(), which reads this. It can't be threaded as a
// per-call parameter instead: Short/flag-help text is baked into the
// cobra command tree at construction time in NewClientRootCommand/
// NewServerRootCommand, before any command's RunE (and thus any
// request-scoped context) exists.
var currentLang = LangRU

// SetLanguage resolves and applies the UI language for the rest of this
// process's lifetime. Call once, before NewClientRootCommand/
// NewServerRootCommand, so Short/flag-help text is built in the right
// language from the start. override (from infra.Config.Lang, i.e. `spur
// lang <ru|en>`) wins if non-empty; otherwise systemLocale (from
// infra.DetectSystemLanguage) decides.
func SetLanguage(override, systemLocale string) {
	if override != "" {
		currentLang = ParseLang(override)
		return
	}
	currentLang = ParseLang(systemLocale)
}

// CurrentLanguage reports the language SetLanguage last resolved to —
// used by `spur lang` (with no argument) to report the effective
// language back to the user.
func CurrentLanguage() Lang {
	return currentLang
}

// ParseLang normalizes a language string case-insensitively. Anything
// other than "ru" falls back to English — the same "English is the
// default, Russian is the explicit case" stance as
// infra.DetectSystemLanguage, so an unrecognized value here (a typo in
// config.json, say) degrades to a still-usable language instead of an
// error.
func ParseLang(s string) Lang {
	if strings.EqualFold(s, "ru") {
		return LangRU
	}
	return LangEN
}
