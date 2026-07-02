package infra

import (
	"os"

	"github.com/rs/zerolog"
)

// NewLogger builds a human-readable logger writing to stderr, timestamped
// to the second. debug lowers the level from info to debug — the server's
// only current use of this, but any future long-running component can
// reuse it the same way.
func NewLogger(debug bool) zerolog.Logger {
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}
	writer := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}
	return zerolog.New(writer).Level(level).With().Timestamp().Logger()
}
