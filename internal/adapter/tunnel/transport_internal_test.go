package tunnel

import (
	"io"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
)

// TestYamuxConfig_ToleratesSlowWrites is a regression test for a real bug
// found live: yamux.DefaultConfig's ConnectionWriteTimeout (10s) is too
// tight for a relay session that's also carrying a large bulk transfer —
// the keepalive ping shares the same send queue as the data, and under
// sustained heavy writes it can lose the race against an in-flight write
// on a connection that is not actually dead. yamux treats that as fatal
// and kills the whole session. See yamuxConfig's doc comment for the
// full story (a 100GB relay transfer died mid-flight this way).
func TestYamuxConfig_ToleratesSlowWrites(t *testing.T) {
	cfg := yamuxConfig()
	def := yamux.DefaultConfig()
	require.Greater(t, cfg.ConnectionWriteTimeout, def.ConnectionWriteTimeout,
		"relay sessions need more slack than yamux's default before a keepalive miss is treated as a dead connection")
	require.Equal(t, io.Discard, cfg.LogOutput, "yamux's own stdlib-log-formatted output shouldn't bypass this codebase's usual error reporting")
}
