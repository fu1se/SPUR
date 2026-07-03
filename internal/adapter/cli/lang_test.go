package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/cli"
)

func TestLangCommand_SetsRuOrEn(t *testing.T) {
	resetLanguage(t)
	cli.SetLanguage("", "ru")

	deps := cli.ClientDependencies{}
	var captured string
	deps.SetLanguage = func(lang string) error {
		captured = lang
		return nil
	}
	cmd := cli.NewClientRootCommand(deps, cli.ClientDefaults{})
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"lang", "en"})

	require.NoError(t, cmd.Execute())
	require.Equal(t, "en", captured)
	require.Contains(t, out.String(), "en")
}

func TestLangCommand_AutoClearsOverride(t *testing.T) {
	resetLanguage(t)
	cli.SetLanguage("", "ru")

	deps := cli.ClientDependencies{}
	var captured *string
	deps.SetLanguage = func(lang string) error {
		captured = &lang
		return nil
	}
	cmd := cli.NewClientRootCommand(deps, cli.ClientDefaults{})
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"lang", "auto"})

	require.NoError(t, cmd.Execute())
	require.NotNil(t, captured)
	require.Equal(t, "", *captured)
}

func TestLangCommand_InvalidArgFails(t *testing.T) {
	resetLanguage(t)
	cli.SetLanguage("", "ru")

	deps := cli.ClientDependencies{
		SetLanguage: func(string) error { return nil },
	}
	cmd := cli.NewClientRootCommand(deps, cli.ClientDefaults{})
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"lang", "de"})

	require.Error(t, cmd.Execute())
}

func TestLangCommand_NoArgsReportsCurrentLanguage(t *testing.T) {
	resetLanguage(t)
	cli.SetLanguage("", "en")

	deps := cli.ClientDependencies{}
	cmd := cli.NewClientRootCommand(deps, cli.ClientDefaults{})
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"lang"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "en")
}
