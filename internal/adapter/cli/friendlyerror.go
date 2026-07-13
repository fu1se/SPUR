package cli

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
)

// Explain turns one of this project's own wrapped errors into a short,
// actionable message for a human, when it recognizes the failure —
// falling back to the original error's message otherwise, so nothing is
// ever hidden.
//
// Every error surfaced through the composition roots (cmd/spur,
// cmd/spur-server) already carries a stable "app: <stage>: ..." /
// "server: <stage>: ..." prefix identifying which step failed (see e.g.
// cmd/spur/tunnel.go's rendezvous — "stun discovery", "exchange
// candidates", "dial control-plane", ...). Explain matches on those
// stage markers, plus the underlying error's real cause where that
// matters (io.EOF, context.DeadlineExceeded, os.IsNotExist, ...), to
// give stage-specific advice instead of a bare Go error string. This is
// deliberately string-matching against our own controlled prefixes (not
// user input, and not third-party error text) rather than threading a
// typed sentinel error through every call site in cmd/*  — pragmatic for
// a UX layer that always has a safe fallback (the original message) when
// it doesn't recognize something, so a wording change elsewhere degrades
// gracefully instead of silently breaking.
func Explain(err error) string {
	if err == nil {
		return ""
	}
	errText := err.Error()
	c := msg()

	switch {
	case strings.Contains(errText, "stun discovery"):
		return friendly(errText, c.ExplainStunHeadline, c.ExplainStunHint)

	case strings.Contains(errText, "exchange candidates"):
		return friendly(errText, c.ExplainExchangeHeadline, c.ExplainExchangeHint)

	case strings.Contains(errText, "dial control-plane"):
		return friendly(errText, c.ExplainDialHeadline, c.ExplainDialHint)

	case strings.Contains(errText, "establish session"):
		return friendly(errText, c.ExplainEstablishHeadline, c.ExplainEstablishHint)

	case strings.Contains(errText, "wait for receiver ack") || strings.Contains(errText, "stream closed") || strings.Contains(errText, "keepalive"):
		return friendly(errText, c.ExplainStreamHeadline, c.ExplainStreamHint)

	case strings.Contains(errText, "invalid invite token") || strings.Contains(errText, "invalid or missing invite token"):
		return friendly(errText, c.ExplainInviteTokenHeadline, c.ExplainInviteTokenHint)

	case strings.Contains(errText, "start desktop server"):
		return friendly(errText, c.ExplainDesktopServerHeadline, c.ExplainDesktopServerHint)

	case strings.Contains(errText, "address already in use"):
		return friendly(errText, c.ExplainAddrInUseHeadline, c.ExplainAddrInUseHint)

	case strings.Contains(errText, "connection refused"):
		return friendly(errText, c.ExplainConnRefusedHeadline, c.ExplainConnRefusedHint)

	case errors.Is(err, fs.ErrNotExist):
		return friendly(errText, c.ExplainNotExistHeadline, c.ExplainNotExistHint)

	case errors.Is(err, fs.ErrPermission):
		return friendly(errText, c.ExplainPermissionHeadline, "")

	case errors.Is(err, context.DeadlineExceeded):
		return friendly(errText, c.ExplainDeadlineHeadline, "")

	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		return friendly(errText, c.ExplainEOFHeadline, "")
	}

	return errText
}

// friendly composes the headline (always shown), an optional hint
// (omitted when empty), and the original technical message (always
// shown, so nothing is ever hidden behind the friendly wording).
func friendly(original, headline, hint string) string {
	s := headline
	if hint != "" {
		s += "\n  " + hint
	}
	s += msg().ExplainTechnicalDetailsPrefix + original
	return s
}
