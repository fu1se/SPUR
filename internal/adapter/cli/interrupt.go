package cli

import (
	"fmt"
	"time"
)

// CtrlCWarningClient/CtrlCWarningServer format the Ctrl+C confirmation
// warning cmd/spur/main.go and cmd/spur-server/main.go print via
// infra.ContextWithConfirmedInterrupt's warn callback. Exported: those
// two are composition roots, outside this package, but the message
// still needs to respect the resolved UI language (see SetLanguage) —
// the same reasoning as Explain being exported for the same two callers.
func CtrlCWarningClient(window time.Duration) string {
	return fmt.Sprintf(msg().CtrlCWarningClient, window)
}

func CtrlCWarningServer(window time.Duration) string {
	return fmt.Sprintf(msg().CtrlCWarningServer, window)
}

// ErrorPrefix is the "Error:"/"Ошибка:" label cmd/spur/main.go and
// cmd/spur-server/main.go print ahead of Explain's output.
func ErrorPrefix() string {
	return msg().ErrorPrefix
}
