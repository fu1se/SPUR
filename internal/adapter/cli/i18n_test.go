package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/cli"
)

// resetLanguage restores the package-level language state (see
// cli.SetLanguage's doc comment on why it's package-level, not
// per-call) after a test that changes it, so tests in this file don't
// leak language state into other tests in this package.
func resetLanguage(t *testing.T) {
	t.Cleanup(func() { cli.SetLanguage("", "ru") })
}

func TestParseLang(t *testing.T) {
	require.Equal(t, cli.LangRU, cli.ParseLang("ru"))
	require.Equal(t, cli.LangRU, cli.ParseLang("RU"))
	require.Equal(t, cli.LangEN, cli.ParseLang("en"))
	require.Equal(t, cli.LangEN, cli.ParseLang("EN"))
	require.Equal(t, cli.LangEN, cli.ParseLang(""))
	require.Equal(t, cli.LangEN, cli.ParseLang("de"))
}

func TestSetLanguage_OverrideWinsOverSystemLocale(t *testing.T) {
	resetLanguage(t)
	cli.SetLanguage("en", "ru")
	require.Equal(t, cli.LangEN, cli.CurrentLanguage())
}

func TestSetLanguage_EmptyOverrideFallsBackToSystemLocale(t *testing.T) {
	resetLanguage(t)
	cli.SetLanguage("", "ru")
	require.Equal(t, cli.LangRU, cli.CurrentLanguage())

	cli.SetLanguage("", "en")
	require.Equal(t, cli.LangEN, cli.CurrentLanguage())
}

// TestSetLanguage_ChangesBuiltCommandText is an end-to-end sanity check
// that SetLanguage actually affects what NewClientRootCommand builds —
// not just the package-level CurrentLanguage() getter.
func TestSetLanguage_ChangesBuiltCommandText(t *testing.T) {
	resetLanguage(t)

	cli.SetLanguage("ru", "")
	ruRoot := cli.NewClientRootCommand(cli.ClientDependencies{}, cli.ClientDefaults{})

	cli.SetLanguage("en", "")
	enRoot := cli.NewClientRootCommand(cli.ClientDependencies{}, cli.ClientDefaults{})

	require.NotEqual(t, ruRoot.Short, enRoot.Short)
	require.Contains(t, ruRoot.Short, "прямое подключение")
	require.Contains(t, enRoot.Short, "direct connection")
}
