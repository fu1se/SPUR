package infra

import (
	"os"
	"strings"
)

// EnvString returns the value of the environment variable key if it is
// set and non-empty, otherwise fallback. Used to layer environment
// variables between the config file and explicit CLI flags: flags always
// win (cobra's own default-vs-explicit-value handling), a set env var
// wins over the config file, and the config file wins over a bare
// zero-value default.
func EnvString(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// EnvBool returns whether the environment variable key is set to a
// recognized truthy ("1", "true", "yes", "on") or falsy ("0", "false",
// "no", "off") value, case-insensitively; otherwise fallback. An unset or
// unrecognized value falls back rather than erroring, since this only
// ever feeds a flag's default — a genuinely invalid value should surface
// as a normal flag-parsing concern, not a silent crash before flags are
// even parsed.
func EnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
