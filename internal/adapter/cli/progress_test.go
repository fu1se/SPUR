package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatETA(t *testing.T) {
	cases := []struct {
		seconds float64
		want    string
	}{
		{5, "~5с"},
		{59, "~59с"},
		{60, "~1м 00с"},
		{125, "~2м 05с"},
		{3599, "~59м 59с"},
		{3600, "~1ч 00м"},
		{7384, "~2ч 03м"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, formatETA(c.seconds))
	}
}

func TestHumanBytes(t *testing.T) {
	require.Equal(t, "0 B", humanBytes(0))
	require.Equal(t, "512 B", humanBytes(512))
	require.Equal(t, "1.0 KiB", humanBytes(1024))
	require.Equal(t, "1.0 MiB", humanBytes(1024*1024))
}
