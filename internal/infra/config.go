package infra

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds default values for the CLI flags a user would otherwise
// have to retype on every invocation (server address, STUN address,
// identity path). Flags always take precedence — Config only fills in
// values the user left unset.
type Config struct {
	Server     string `json:"server,omitempty"`
	StunServer string `json:"stun_server,omitempty"`
	Identity   string `json:"identity,omitempty"`

	// Lang is the user's explicit UI language override ("ru"/"en"), set
	// via `spur lang <ru|en>` and persisted here. Empty means "no
	// override" — the language is auto-detected from the system locale
	// instead (see DetectSystemLanguage); `spur lang auto` clears this
	// field back to empty rather than writing a sentinel value, so a
	// config file with no lang key at all behaves identically to one
	// that explicitly asked for auto-detection.
	Lang string `json:"lang,omitempty"`
}

// DefaultConfigPath is where LoadConfig looks by default.
func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("infra: user config dir: %w", err)
	}
	return filepath.Join(dir, "spur", "config.json"), nil
}

// DefaultServerStatePath is where the server's SQLite database
// (adapter/repository/sqlite) lives by default.
func DefaultServerStatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("infra: user config dir: %w", err)
	}
	return filepath.Join(dir, "spur", "state.db"), nil
}

// LoadConfig reads path as JSON. A missing file is not an error: it just
// means every field stays at its zero value, so every flag keeps behaving
// as if no config file exists at all — the same "optional, additive"
// stance identity/known_servers files already take.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("infra: read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("infra: parse config %s: %w", path, err)
	}
	return cfg, nil
}

// SaveConfig writes cfg to path as JSON, creating the parent directory if
// needed. Used by `spur lang` to persist a language override — the only
// thing that writes this file; every other field is still edited by hand
// or left absent. Atomic write (temp file in the same directory +
// rename): a plain os.WriteFile truncates before writing, so a
// concurrent reader (a second spur process starting up at the same
// moment) could otherwise observe a partially-written file — same
// reasoning as saveTrustStore in tofu.go.
func SaveConfig(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("infra: create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("infra: encode config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("infra: create config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("infra: write config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("infra: close config temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("infra: chmod config temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("infra: replace config: %w", err)
	}
	return nil
}
