package infra

import (
	"os"
	"strings"
)

// DetectSystemLanguage inspects the standard POSIX locale environment
// variables, in their usual precedence order (LC_ALL overrides
// LC_MESSAGES overrides LANG), and returns "ru" if the first non-empty
// one names a Russian locale (e.g. "ru_RU.UTF-8", "ru"), "en" otherwise.
// English, not Russian, is the fallback for anything else (unset,
// unparsable, or a locale that isn't Russian) — this only decides a
// default; SaveLanguage/Config.Lang lets a user override it explicitly.
func DetectSystemLanguage() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(env)
		if v == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(v), "ru") {
			return "ru"
		}
		return "en"
	}
	return "en"
}
