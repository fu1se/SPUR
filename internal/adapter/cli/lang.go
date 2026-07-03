package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newLangCommand is "spur lang [ru|en|auto]": with no argument, reports
// the language currently in effect and how it was decided; with an
// argument, persists a new override ("auto" clears it back to
// system-locale detection) via deps.SetLanguage for future invocations.
// It cannot change the language of the invocation that's setting it —
// SetLanguage (the package-level function) already ran, and this
// command's own Short/flag text was already built in whatever language
// was in effect at the time.
func newLangCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	return &cobra.Command{
		Use:   "lang [ru|en|auto]",
		Short: msg().LangShort,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if defaults.Lang != "" {
					cmd.Printf(msg().LangCurrentOverride, CurrentLanguage())
				} else {
					cmd.Printf(msg().LangCurrentAuto, CurrentLanguage())
				}
				return nil
			}

			switch args[0] {
			case "auto":
				if err := deps.SetLanguage(""); err != nil {
					return err
				}
				cmd.Print(msg().LangAutoConfirm)
			case "ru", "en":
				if err := deps.SetLanguage(args[0]); err != nil {
					return err
				}
				cmd.Printf(msg().LangSetConfirm, args[0])
			default:
				return fmt.Errorf(msg().LangInvalidArg, args[0])
			}
			return nil
		},
	}
}
