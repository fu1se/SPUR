package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// pairingToFlagHelp is the shared --to flag description text for every
// command that accepts either a full peer ID, a short pairing code, or
// (left empty) host mode.
func pairingToFlagHelp(subject string) string {
	return fmt.Sprintf(msg().PairingToFlagHelp, subject, msg().PairingCodeTTLHint)
}

// newCodePrinter returns an OnCodeFunc that prints a freshly minted
// pairing code prominently — this is host mode, entered whenever the
// user left --to empty. Written to cmd's output so `spur send`/`spur
// receive`/etc without --to still work the same way any other status
// message does (cmd.Printf, not fmt.Println directly, matching this
// package's existing convention throughout).
func newCodePrinter(cmd *cobra.Command) OnCodeFunc {
	return func(code string) {
		cmd.Printf(msg().CodePrintedLine1, code)
		cmd.Printf(msg().CodePrintedLine2, msg().PairingCodeTTLHint)
	}
}
