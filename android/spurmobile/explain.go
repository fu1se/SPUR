package spurmobile

import (
	"errors"

	"github.com/fu1se/spur/internal/adapter/cli"
)

// explain re-wraps a Go error as a friendly, human-readable one before it
// crosses into Kotlin. gomobile only carries an error's Error() string
// across the JNI boundary (as Throwable.getMessage()) — nothing survives
// on the Kotlin side to post-process later, so this is the only chance
// to make the message friendly. Reuses the exact same catalog the
// desktop CLI's cli.Explain already has (STUN timeout, exchange-
// candidates EOF, wrong invite token, ...) rather than duplicating it:
// android/spurmobile is free to import any internal/... package, the
// dependency restriction only runs the other way.
func explain(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(cli.Explain(err))
}
