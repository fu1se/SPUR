package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// newRegisterCommand exercises the Phase 2 control-plane registration flow
// end to end: dial the server, register, print back what the server saw.
// It exists to validate the control-plane independently of the data-plane
// modes (connect/join) that later phases build on top of it.
func newRegisterCommand(deps Dependencies) *cobra.Command {
	var serverAddr string

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
			cmd.Printf("peer-id: %s\n", result.PeerID)
			cmd.Printf("observed-address: %s\n", result.ObservedAddress)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", "", "адрес rendezvous-сервера")

	return cmd
}
