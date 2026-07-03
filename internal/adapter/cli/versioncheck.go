package cli

import "github.com/spf13/cobra"

// newVersionWarningPrinter returns a VersionMismatchFunc that warns on
// stderr that this client and the server it just talked to are running
// different build versions — best-effort, not a hard failure: some
// features negotiated between them (a newer wire message, say) might not
// work as expected, but plenty of version skews are perfectly compatible
// too, so this never blocks the command from proceeding.
func newVersionWarningPrinter(cmd *cobra.Command) VersionMismatchFunc {
	return func(clientVersion, serverVersion string) {
		cmd.Printf("Внимание: версия клиента (%s) отличается от версии сервера (%s) — некоторые функции могут работать некорректно. Обновите обе стороны до одной версии, если возникнут проблемы.\n", clientVersion, serverVersion)
	}
}
