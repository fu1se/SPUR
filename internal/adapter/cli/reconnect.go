package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// OnReconnectFunc is called when an established tunnel was lost and the
// command is about to rendezvous again after delay — long-lived commands
// (connect/expose, file transfers) survive network drops by reconnecting
// instead of dying (see rendezvous.RunPersistent). Mirrors
// rendezvous.OnReconnectFunc's shape rather than importing it, same
// reasoning as ProgressFunc/OnCodeFunc. nil is valid and means "don't
// report".
type OnReconnectFunc func(cause error, delay time.Duration)

// newReconnectPrinter returns an OnReconnectFunc that tells the user the
// connection dropped and when the next attempt happens. The cause is
// printed raw (single line), not run through Explain: a reconnecting
// session may emit this repeatedly, and a multi-line explanation with
// hints on every attempt would drown the terminal — Explain still runs on
// the final error if the command ultimately gives up.
func newReconnectPrinter(cmd *cobra.Command) OnReconnectFunc {
	return func(cause error, delay time.Duration) {
		cmd.Printf(msg().ReconnectNotice, cause, delay.Round(time.Second))
	}
}
