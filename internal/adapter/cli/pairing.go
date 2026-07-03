package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// pairingCodeTTLHint is a human-readable duration matching
// usecase.PairingCodeTTL. Duplicated as a plain string rather than
// imported: cli must not depend on usecase (see ProgressFunc's doc
// comment for the same rule applied elsewhere) — this is purely a
// user-facing hint, not logic, so the duplication is low-risk, but keep
// it in sync if PairingCodeTTL ever changes.
const pairingCodeTTLHint = "10 минут"

// pairingToFlagHelp is the shared --to flag description text for every
// command that accepts either a full peer ID, a short pairing code, or
// (left empty) host mode.
func pairingToFlagHelp(subject string) string {
	return fmt.Sprintf("идентификатор или код %s; не указан — сгенерировать свой код и ждать подключения (см. 'spur whoami' для постоянного ID, код — одноразовый на %s)", subject, pairingCodeTTLHint)
}

// newCodePrinter returns an OnCodeFunc that prints a freshly minted
// pairing code prominently — this is host mode, entered whenever the
// user left --to empty. Written to cmd's output so `spur send`/`spur
// receive`/etc without --to still work the same way any other status
// message does (cmd.Printf, not fmt.Println directly, matching this
// package's existing convention throughout).
func newCodePrinter(cmd *cobra.Command) OnCodeFunc {
	return func(code string) {
		cmd.Printf("Код для подключения: %s\n", code)
		cmd.Printf("Сообщите его собеседнику — он должен указать этот код в --to. Ждём подключения (до %s)...\n", pairingCodeTTLHint)
	}
}
