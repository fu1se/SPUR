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
}

// DefaultConfigPath is where LoadConfig looks by default.
func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("infra: user config dir: %w", err)
	}
	return filepath.Join(dir, "localizator", "config.json"), nil
}

// DefaultServerStatePath is where the server's SQLite database
// (adapter/repository/sqlite) lives by default.
func DefaultServerStatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("infra: user config dir: %w", err)
	}
	return filepath.Join(dir, "localizator", "state.db"), nil
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
