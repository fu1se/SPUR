package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/domain"
)

func TestSession_Established(t *testing.T) {
	cases := []struct {
		state domain.SessionState
		want  bool
	}{
		{domain.SessionPending, false},
		{domain.SessionPunching, false},
		{domain.SessionFailed, false},
		{domain.SessionEstablishedP2P, true},
		{domain.SessionEstablishedRelay, true},
	}

	for _, tc := range cases {
		s := domain.Session{State: tc.state}
		require.Equal(t, tc.want, s.Established(), "state=%s", tc.state)
	}
}
