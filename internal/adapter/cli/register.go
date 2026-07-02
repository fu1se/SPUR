package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// newRegisterCommand exercises the Phase 2 control-plane registration flow
// end to end: dial the server, register, print back what the server saw.
// It exists to validate the control-plane independently of the data-plane
// modes (connect/join) that later phases build on top of it.
func newRegisterCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var serverAddr = defaults.Server

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Зарегистрироваться на rendezvous-сервере и показать наблюдаемый им адрес",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" {
				return errors.New("register: укажите --server")
			}
			result, err := deps.Register(cmd.Context(), serverAddr)
			if err != nil {
				return err
			}
			cmd.Printf("observed-address: %s\n", result.ObservedAddress)
			// peer-id specifically goes to stdout via fmt.Fprintln, not
			// cmd.Printf (which defaults to stderr, see whoami.go's same
			// comment): register is the other command that surfaces a
			// peer-id, and someone reasonably expecting
			// `id=$(spur register --server ...)` to work the same way
			// `id=$(spur whoami)` does deserves that to actually work.
			fmt.Fprintln(cmd.OutOrStdout(), result.PeerID)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера")

	return cmd
}
