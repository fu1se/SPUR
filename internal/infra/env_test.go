package infra_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/infra"
)

func TestEnvString_UsesEnvWhenSet(t *testing.T) {
	t.Setenv("SPUR_TEST_STRING", "from-env")
	require.Equal(t, "from-env", infra.EnvString("SPUR_TEST_STRING", "fallback"))
}

func TestEnvString_FallsBackWhenUnset(t *testing.T) {
	require.Equal(t, "fallback", infra.EnvString("SPUR_TEST_STRING_UNSET", "fallback"))
}

func TestEnvString_FallsBackWhenEmpty(t *testing.T) {
	t.Setenv("SPUR_TEST_STRING_EMPTY", "")
	require.Equal(t, "fallback", infra.EnvString("SPUR_TEST_STRING_EMPTY", "fallback"))
}

func TestEnvBool_RecognizesTruthyAndFalsyValues(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv("SPUR_TEST_BOOL", v)
		require.True(t, infra.EnvBool("SPUR_TEST_BOOL", false), "expected %q to be truthy", v)
	}
	for _, v := range []string{"0", "false", "FALSE", "no", "off"} {
		t.Setenv("SPUR_TEST_BOOL", v)
		require.False(t, infra.EnvBool("SPUR_TEST_BOOL", true), "expected %q to be falsy", v)
	}
}

func TestEnvBool_FallsBackWhenUnsetOrUnrecognized(t *testing.T) {
	require.True(t, infra.EnvBool("SPUR_TEST_BOOL_UNSET", true))

	t.Setenv("SPUR_TEST_BOOL_GARBAGE", "maybe")
	require.True(t, infra.EnvBool("SPUR_TEST_BOOL_GARBAGE", true))
	require.False(t, infra.EnvBool("SPUR_TEST_BOOL_GARBAGE", false))
}
